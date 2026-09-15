package service

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/givetrack/givetrack/internal/constants"
	"github.com/givetrack/givetrack/internal/model"
	"github.com/givetrack/givetrack/internal/repository"
)

// applyOK 提交申请，失败即终止。
func (f *fundTestFixture) applyOK(t *testing.T, amount float64, purpose string) *model.DisbursementApplication {
	t.Helper()
	app, err := f.svc.Apply(f.orgUserID, f.projectID, ApplyInput{Amount: amount, Purpose: purpose})
	if err != nil {
		t.Fatalf("apply amount=%.2f: unexpected error: %v", amount, err)
	}
	return app
}

// 主流程：申请 → 审核 → 生成唯一拨付单 → 回填凭证 → 汇总 → 公示。
func TestDisbursementMainFlow(t *testing.T) {
	f := newFundFixture(t)

	// 1. 分批申请：6000 + 3000 = 9000，未超过已筹 10000。
	app1 := f.applyOK(t, 6000, "采购图书")
	app2 := f.applyOK(t, 3000, "配送与安装")
	if app1.Status != constants.FundApplyPending || app2.Status != constants.FundApplyPending {
		t.Fatalf("new applications should be pending")
	}
	if app1.BatchNo == "" || app2.BatchNo == "" {
		t.Fatalf("batch no should be generated automatically")
	}

	// 待审占用额度：9000，剩余 1000。
	sum, err := f.svc.GetSummary(f.projectID)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if !almostEqual(sum.OccupiedAmount, 9000) || !almostEqual(sum.PendingAmount, 9000) ||
		!almostEqual(sum.DisbursedAmount, 0) || !almostEqual(sum.RemainingAmount, 1000) {
		t.Fatalf("unexpected summary after apply: %+v", sum)
	}

	// 2. 审核通过 app1，生成唯一拨付单。
	a1, order1, err := f.svc.Review(f.adminID, app1.ID, ReviewInput{Approve: true, Comment: "同意"})
	if err != nil {
		t.Fatalf("review approve: %v", err)
	}
	if a1.Status != constants.FundApplyApproved || order1 == nil {
		t.Fatalf("approve should set status and create order")
	}
	if len(order1.OrderNo) == 0 {
		t.Fatalf("order number must be generated")
	}
	// 审核通过即完成拨付：拨付单状态为 paid 且记录拨付时间。
	if order1.Status != constants.DisbursementPaid || order1.PaidAt == nil {
		t.Fatalf("order should be paid on approval, status=%s", order1.Status)
	}
	// 一个申请至多一张拨付单：唯一索引。
	dupOrder := &model.DisbursementOrder{OrderNo: order1.OrderNo + "x", ApplicationID: app1.ID, ProjectID: f.projectID, OrgID: f.orgID, Amount: 6000}
	if err := f.db.Create(dupOrder).Error; err == nil {
		t.Fatalf("second order for same application must be rejected by unique index")
	}

	// 3. 驳回 app2，释放额度。
	a2, order2, err := f.svc.Review(f.adminID, app2.ID, ReviewInput{Approve: false, Comment: "材料待补"})
	if err != nil || order2 != nil || a2.Status != constants.FundApplyRejected {
		t.Fatalf("reject should release quota and create no order, got err=%v order=%v status=%s", err, order2, a2.Status)
	}

	// 4. 回填支出凭证（分两笔），累计不得超过拨付单 6000。
	v1, err := f.svc.AddVoucher(f.orgUserID, app1.ID, VoucherInput{
		Amount: 3500, Category: "物资采购", Usage: "课外读物 3000 册", InvoiceNo: "INV001", SpentAt: "2026-09-01",
	})
	if err != nil {
		t.Fatalf("add voucher 1: %v", err)
	}
	if _, err := f.svc.AddVoucher(f.orgUserID, app1.ID, VoucherInput{
		Amount: 2500, Category: "物流安装", Usage: "书架配送与安装",
	}); err != nil {
		t.Fatalf("add voucher 2: %v", err)
	}
	// 拨付单在审核通过时即已置为 paid；回填凭证不改变该状态。
	paid, err := repository.NewDisbursementOrderRepository(f.db).FindByID(order1.ID)
	if err != nil {
		t.Fatalf("reload order: %v", err)
	}
	if paid.Status != constants.DisbursementPaid || paid.PaidAt == nil {
		t.Fatalf("order should remain paid, status=%s", paid.Status)
	}

	// 5. 凭证刚回填尚未核验：捐赠人可见口径的已用金额为 0，公示凭证为空。
	sum, err = f.svc.GetSummary(f.projectID)
	if err != nil {
		t.Fatalf("summary after review: %v", err)
	}
	if !almostEqual(sum.DisbursedAmount, 6000) || !almostEqual(sum.UsedAmount, 0) ||
		!almostEqual(sum.OccupiedAmount, 6000) || !almostEqual(sum.RemainingAmount, 4000) ||
		!almostEqual(sum.PendingAmount, 0) {
		t.Fatalf("unchecked vouchers must not count as used: %+v", sum)
	}

	orders, vouchers, psum, err := f.svc.PublicFunds(f.projectID)
	if err != nil {
		t.Fatalf("public funds: %v", err)
	}
	if len(orders) != 1 || orders[0].OrderNo != order1.OrderNo {
		t.Fatalf("public should only see approved order, got %d", len(orders))
	}
	if len(vouchers) != 0 {
		t.Fatalf("public must not see unchecked vouchers, got %d", len(vouchers))
	}
	if !almostEqual(psum.UsedAmount, 0) {
		t.Fatalf("public used amount should be 0 before checking: %+v", psum)
	}

	// 6. 平台核验第一张凭证（3500）：已用变为 3500，公示仅见 1 张。
	if _, err := f.svc.CheckVoucher(f.adminID, constants.RoleAdmin, v1.ID); err != nil {
		t.Fatalf("check voucher 1: %v", err)
	}
	sum, err = f.svc.GetSummary(f.projectID)
	if err != nil || !almostEqual(sum.UsedAmount, 3500) {
		t.Fatalf("after checking v1 used should be 3500: %+v err=%v", sum, err)
	}
	_, vouchers, psum, err = f.svc.PublicFunds(f.projectID)
	if err != nil || len(vouchers) != 1 || !almostEqual(psum.UsedAmount, 3500) {
		t.Fatalf("public should show 1 checked voucher / used 3500, got %d %+v err=%v", len(vouchers), psum, err)
	}

	// 7. 核验第二张凭证（2500）：已用 6000，公示见 2 张；剩余口径仍由占用决定 = 4000。
	appDetail, allVouchers, err := f.svc.GetApplication(f.orgUserID, constants.RoleOrg, app1.ID)
	if err != nil {
		t.Fatalf("get application vouchers: %v", err)
	}
	_ = appDetail
	if len(allVouchers) != 2 {
		t.Fatalf("org should see both vouchers incl. unchecked, got %d", len(allVouchers))
	}
	var uncheckedID uint
	for _, vv := range allVouchers {
		if vv.Status != constants.VoucherChecked {
			uncheckedID = vv.ID
		}
	}
	if _, err := f.svc.CheckVoucher(f.adminID, constants.RoleAdmin, uncheckedID); err != nil {
		t.Fatalf("check voucher 2: %v", err)
	}
	sum, _ = f.svc.GetSummary(f.projectID)
	if !almostEqual(sum.UsedAmount, 6000) || !almostEqual(sum.RemainingAmount, 4000) {
		t.Fatalf("after both checked used=6000 remaining=4000: %+v", sum)
	}
	_, vouchers, psum, _ = f.svc.PublicFunds(f.projectID)
	if len(vouchers) != 2 || !almostEqual(psum.UsedAmount, 6000) {
		t.Fatalf("public should show 2 checked vouchers / used 6000, got %d %+v", len(vouchers), psum)
	}

	// 平台可追溯审核意见、审核人与发生时间。
	if a1.ReviewComment != "同意" || a1.ReviewerID != f.adminID || a1.ReviewedAt == nil {
		t.Fatalf("audit trail must keep comment/reviewer/reviewedAt: %+v", a1)
	}
	if a2.ReviewComment != "材料待补" {
		t.Fatalf("rejection comment should be persisted")
	}
	_ = v1
}

// 超额申请：待审 + 已批超过已筹即拒绝。
func TestApplyQuotaExceeded(t *testing.T) {
	f := newFundFixture(t)
	f.applyOK(t, 7000, "第一批")

	// 7000 + 4000 = 11000 > 10000，拒绝。
	_, err := f.svc.Apply(f.orgUserID, f.projectID, ApplyInput{Amount: 4000, Purpose: "超额批次"})
	if !errors.Is(err, ErrFundQuotaExceeded) {
		t.Fatalf("expected ErrFundQuotaExceeded, got %v", err)
	}

	// 边界：恰好 3000（7000+3000=10000）应通过。
	f.applyOK(t, 3000, "恰好满额")
}

// 重复审核：同一申请审核两次必须拒绝。
func TestDuplicateReviewRejected(t *testing.T) {
	f := newFundFixture(t)
	app := f.applyOK(t, 2000, "用款")
	if _, _, err := f.svc.Review(f.adminID, app.ID, ReviewInput{Approve: true}); err != nil {
		t.Fatalf("first review: %v", err)
	}
	if _, _, err := f.svc.Review(f.adminID, app.ID, ReviewInput{Approve: true}); !errors.Is(err, ErrApplicationNotPending) {
		t.Fatalf("second review must be ErrApplicationNotPending, got %v", err)
	}
	// 驳回后再次审核同样被拒。
	app2 := f.applyOK(t, 1000, "用款2")
	if _, _, err := f.svc.Review(f.adminID, app2.ID, ReviewInput{Approve: false}); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if _, _, err := f.svc.Review(f.adminID, app2.ID, ReviewInput{Approve: true}); !errors.Is(err, ErrApplicationNotPending) {
		t.Fatalf("review after reject must be rejected, got %v", err)
	}
}

// 结项后再申请/再拨均拒绝；有待审申请时不允许结项。
func TestSettledProjectRejectsDisbursement(t *testing.T) {
	f := newFundFixture(t)

	// 有待审申请时不能结项。
	pending := f.applyOK(t, 2000, "待结项审批")
	if err := f.svc.SettleProject(f.orgUserID, f.projectID); !errors.Is(err, ErrHasPendingApplication) {
		t.Fatalf("settle with pending application should fail, got %v", err)
	}

	// 审核通过后才能结项。
	if _, _, err := f.svc.Review(f.adminID, pending.ID, ReviewInput{Approve: true}); err != nil {
		t.Fatalf("review: %v", err)
	}
	if err := f.svc.SettleProject(f.orgUserID, f.projectID); err != nil {
		t.Fatalf("settle: %v", err)
	}

	// 结项后提交申请被拒。
	if _, err := f.svc.Apply(f.orgUserID, f.projectID, ApplyInput{Amount: 100, Purpose: "结项后申请"}); !errors.Is(err, ErrProjectSettled) {
		t.Fatalf("apply after settle must be ErrProjectSettled, got %v", err)
	}

	// 结项后审核一个残留 pending 也应被拒（构造一个未结项项目无法复用，这里直接验证重复结项）。
	if err := f.svc.SettleProject(f.orgUserID, f.projectID); !errors.Is(err, ErrProjectSettled) {
		t.Fatalf("double settle must fail, got %v", err)
	}
}

// 结项后审核待批申请也应被拒绝（拨款发生在结项之后）。
func TestReviewAfterSettleRejected(t *testing.T) {
	f := newFundFixture(t)

	// 项目 A：结项。
	settledProject := &model.Project{OrganizationID: f.orgID, Title: "将结项", Category: "education",
		TargetAmount: 5000, CurrentAmount: 5000, Status: constants.ProjectCompleted}
	if err := f.db.Create(settledProject).Error; err != nil {
		t.Fatalf("create settled project: %v", err)
	}
	now := time.Now()
	settledProject.SettledAt = &now
	if err := f.db.Save(settledProject).Error; err != nil {
		t.Fatalf("settle: %v", err)
	}
	// 直接落库一个挂在已结项项目上的 pending 申请，模拟“先申请、后结项、再审核”。
	app := &model.DisbursementApplication{ProjectID: settledProject.ID, OrgID: f.orgID, ApplicantID: f.orgUserID,
		Amount: 1000, Purpose: "结项后才被审", Status: constants.FundApplyPending}
	if err := f.db.Create(app).Error; err != nil {
		t.Fatalf("create app: %v", err)
	}
	if _, _, err := f.svc.Review(f.adminID, app.ID, ReviewInput{Approve: true}); !errors.Is(err, ErrProjectSettled) {
		t.Fatalf("review on settled project must be ErrProjectSettled, got %v", err)
	}
}

// 凭证超额：同一拨付单累计支出超过拨付金额被拒；未审核申请不可回填。
func TestVoucherRules(t *testing.T) {
	f := newFundFixture(t)

	// 未审核申请不能回填。
	notApproved := f.applyOK(t, 2000, "待审")
	if _, err := f.svc.AddVoucher(f.orgUserID, notApproved.ID, VoucherInput{Amount: 10, Category: "x", Usage: "yy"}); !errors.Is(err, ErrApplicationNotApproved) {
		t.Fatalf("voucher on pending application must fail, got %v", err)
	}

	app := f.applyOK(t, 5000, "已批")
	if _, _, err := f.svc.Review(f.adminID, app.ID, ReviewInput{Approve: true}); err != nil {
		t.Fatalf("review: %v", err)
	}
	if _, err := f.svc.AddVoucher(f.orgUserID, app.ID, VoucherInput{Amount: 4500, Category: "物资", Usage: "采购"}); err != nil {
		t.Fatalf("voucher within limit: %v", err)
	}
	// 4500 + 800 = 5300 > 5000，拒绝。
	if _, err := f.svc.AddVoucher(f.orgUserID, app.ID, VoucherInput{Amount: 800, Category: "物流", Usage: "配送"}); !errors.Is(err, ErrVoucherExceedsOrder) {
		t.Fatalf("over-limit voucher must be rejected, got %v", err)
	}
	// 恰好补足 500 应通过。
	if _, err := f.svc.AddVoucher(f.orgUserID, app.ID, VoucherInput{Amount: 500, Category: "物流", Usage: "配送"}); err != nil {
		t.Fatalf("voucher filling exact limit should pass: %v", err)
	}
}

// 非本项目所属组织不能申请或回填。
func TestCrossOrgForbidden(t *testing.T) {
	f := newFundFixture(t)
	if _, err := f.svc.Apply(f.otherOrgUserID, f.projectID, ApplyInput{Amount: 100, Purpose: "别人项目"}); !errors.Is(err, ErrNotProjectOwner) {
		t.Fatalf("other org apply must be forbidden, got %v", err)
	}
	app := f.applyOK(t, 1000, "本组织")
	if _, _, err := f.svc.Review(f.adminID, app.ID, ReviewInput{Approve: true}); err != nil {
		t.Fatalf("review: %v", err)
	}
	if _, err := f.svc.AddVoucher(f.otherOrgUserID, app.ID, VoucherInput{Amount: 10, Category: "x", Usage: "yy"}); !errors.Is(err, ErrNotProjectOwner) {
		t.Fatalf("other org voucher must be forbidden, got %v", err)
	}
}

// 未通过审核的项目不可申请用款。
func TestApplyOnUnapprovedProject(t *testing.T) {
	f := newFundFixture(t)
	pendingProject := &model.Project{OrganizationID: f.orgID, Title: "待审核项目", Category: "education",
		TargetAmount: 1000, CurrentAmount: 0, Status: constants.ProjectPending}
	if err := f.db.Create(pendingProject).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := f.svc.Apply(f.orgUserID, pendingProject.ID, ApplyInput{Amount: 100, Purpose: "未过审项目用款"}); !errors.Is(err, ErrProjectNotFundable) {
		t.Fatalf("apply on pending project must fail, got %v", err)
	}
}

// 并发提交：多笔申请并发提交，总额超过已筹时只允许一部分成功，
// 已成功（待审+已批）总额绝不能突破已筹资金。
func TestConcurrentApplyQuotaSafe(t *testing.T) {
	f := newFundFixture(t)
	const n = 10
	const each = 1500.0 // 合计 15000，已筹仅 10000，至多 6 笔成功（9000）

	var wg sync.WaitGroup
	errs := make([]error, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i] = f.svc.Apply(f.orgUserID, f.projectID, ApplyInput{Amount: each, Purpose: "并发申请"})
		}(i)
	}
	close(start)
	wg.Wait()

	var success, rejected int
	for _, e := range errs {
		switch {
		case e == nil:
			success++
		case errors.Is(e, ErrFundQuotaExceeded):
			rejected++
		default:
			t.Fatalf("unexpected concurrent error: %v", e)
		}
	}
	if success == 0 || success >= n {
		t.Fatalf("expected some but not all to succeed, success=%d rejected=%d", success, rejected)
	}
	if success+rejected != n {
		t.Fatalf("all goroutines should finish, got %d", success+rejected)
	}

	occupied, err := repository.NewFundApplicationRepository(f.db).OccupyingAmount(f.projectID)
	if err != nil {
		t.Fatalf("occupied: %v", err)
	}
	if occupied > 10000+constants.AmountEpsilon {
		t.Fatalf("occupied amount breached raised funds: %.2f", occupied)
	}
	if !almostEqual(occupied, float64(success)*each) {
		t.Fatalf("occupied %.2f != success count %d * %.2f", occupied, success, each)
	}
}

// 并发审核同一申请：至多一个成功，且只生成一张拨付单。
func TestConcurrentDuplicateReviewSafe(t *testing.T) {
	f := newFundFixture(t)
	app := f.applyOK(t, 2000, "并发审核")

	const n = 5
	var wg sync.WaitGroup
	errs := make([]error, n)
	orders := make([]*model.DisbursementOrder, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, orders[i], errs[i] = f.svc.Review(f.adminID, app.ID, ReviewInput{Approve: true})
		}(i)
	}
	close(start)
	wg.Wait()

	var approved int
	for i, e := range errs {
		if e == nil {
			approved++
			if orders[i] == nil {
				t.Fatalf("successful review must return an order")
			}
		} else if !errors.Is(e, ErrApplicationNotPending) {
			t.Fatalf("unexpected review error: %v", e)
		}
	}
	if approved != 1 {
		t.Fatalf("exactly one concurrent review should succeed, got %d", approved)
	}
	var orderCount int64
	if err := f.db.Model(&model.DisbursementOrder{}).Where("application_id = ?", app.ID).Count(&orderCount).Error; err != nil {
		t.Fatalf("count orders: %v", err)
	}
	if orderCount != 1 {
		t.Fatalf("exactly one order allowed, got %d", orderCount)
	}
}

// 驳回申请会释放额度，之后可再申请同等金额。
func TestRejectReleasesQuota(t *testing.T) {
	f := newFundFixture(t)
	app := f.applyOK(t, 10000, "全额占用")
	if _, err := f.svc.Apply(f.orgUserID, f.projectID, ApplyInput{Amount: 1, Purpose: "再多一块"}); !errors.Is(err, ErrFundQuotaExceeded) {
		t.Fatalf("quota should be fully occupied, got %v", err)
	}
	if _, _, err := f.svc.Review(f.adminID, app.ID, ReviewInput{Approve: false}); err != nil {
		t.Fatalf("reject: %v", err)
	}
	// 释放后可重新申请满额。
	f.applyOK(t, 10000, "重新申请")
}

func almostEqual(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < constants.AmountEpsilon
}

// makeCheckedVoucher 创建：申请→通过→回填一张待核验凭证，返回凭证。
func (f *fundTestFixture) makeUncheckedVoucher(t *testing.T, amount float64) *model.ExpenseVoucher {
	t.Helper()
	app := f.applyOK(t, amount, "核验测试用款")
	if _, _, err := f.svc.Review(f.adminID, app.ID, ReviewInput{Approve: true}); err != nil {
		t.Fatalf("review: %v", err)
	}
	v, err := f.svc.AddVoucher(f.orgUserID, app.ID, VoucherInput{
		Amount: amount, Category: "物资", Usage: "核验测试",
	})
	if err != nil {
		t.Fatalf("add voucher: %v", err)
	}
	return v
}

// 非平台管理员无权核验凭证（组织、普通用户均被拒）。
func TestCheckVoucherForbidden(t *testing.T) {
	f := newFundFixture(t)
	v := f.makeUncheckedVoucher(t, 1000)

	if _, err := f.svc.CheckVoucher(f.orgUserID, constants.RoleOrg, v.ID); !errors.Is(err, ErrVoucherCheckForbidden) {
		t.Fatalf("org must not check vouchers, got %v", err)
	}
	// 越权尝试不得改变凭证状态，也不得计入已用。
	got, err := repository.NewExpenseVoucherRepository(f.db).FindByID(v.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Status != constants.VoucherPending {
		t.Fatalf("voucher must remain pending after forbidden attempt, got %s", got.Status)
	}
	sum, _ := f.svc.GetSummary(f.projectID)
	if !almostEqual(sum.UsedAmount, 0) {
		t.Fatalf("used amount must stay 0, got %.2f", sum.UsedAmount)
	}
}

// 重复核验与核验后再次核验都必须返回明确冲突。
func TestCheckVoucherDuplicateRejected(t *testing.T) {
	f := newFundFixture(t)
	v := f.makeUncheckedVoucher(t, 1000)

	checked, err := f.svc.CheckVoucher(f.adminID, constants.RoleAdmin, v.ID)
	if err != nil {
		t.Fatalf("first check: %v", err)
	}
	if checked.Status != constants.VoucherChecked || checked.CheckedAt == nil || checked.CheckerID != f.adminID {
		t.Fatalf("first check must record status/checker/time: %+v", checked)
	}
	// 再次核验 → 冲突。
	if _, err := f.svc.CheckVoucher(f.adminID, constants.RoleAdmin, v.ID); !errors.Is(err, ErrVoucherAlreadyChecked) {
		t.Fatalf("duplicate check must be ErrVoucherAlreadyChecked, got %v", err)
	}
	// 第三次同样冲突。
	if _, err := f.svc.CheckVoucher(f.adminID, constants.RoleAdmin, v.ID); !errors.Is(err, ErrVoucherAlreadyChecked) {
		t.Fatalf("repeat check after checked must conflict, got %v", err)
	}
	// 已用金额只被计入一次（1000，而非 2000/3000）。
	sum, _ := f.svc.GetSummary(f.projectID)
	if !almostEqual(sum.UsedAmount, 1000) {
		t.Fatalf("used amount must count voucher once, got %.2f", sum.UsedAmount)
	}
}

// 并发核验同一凭证：恰好一次成功，其余全部冲突，金额只计入一次。
func TestConcurrentCheckVoucherSafe(t *testing.T) {
	f := newFundFixture(t)
	v := f.makeUncheckedVoucher(t, 2000)

	const n = 6
	var wg sync.WaitGroup
	errs := make([]error, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i] = f.svc.CheckVoucher(f.adminID, constants.RoleAdmin, v.ID)
		}(i)
	}
	close(start)
	wg.Wait()

	var ok, conflict int
	for _, e := range errs {
		switch {
		case e == nil:
			ok++
		case errors.Is(e, ErrVoucherAlreadyChecked):
			conflict++
		default:
			t.Fatalf("unexpected check error: %v", e)
		}
	}
	if ok != 1 || conflict != n-1 {
		t.Fatalf("exactly 1 success and %d conflicts expected, got ok=%d conflict=%d", n-1, ok, conflict)
	}

	var checkedCount int64
	if err := f.db.Model(&model.ExpenseVoucher{}).
		Where("id = ? AND status = ?", v.ID, constants.VoucherChecked).Count(&checkedCount).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if checkedCount != 1 {
		t.Fatalf("voucher must be checked exactly once, got %d", checkedCount)
	}
	sum, _ := f.svc.GetSummary(f.projectID)
	if !almostEqual(sum.UsedAmount, 2000) {
		t.Fatalf("used amount must count once under concurrency, got %.2f", sum.UsedAmount)
	}
}

// 凭证处于异常（非 pending/checked）状态时，核验必须返回冲突，不得当成待核验放行。
func TestCheckVoucherAbnormalStatusRejected(t *testing.T) {
	f := newFundFixture(t)
	v := f.makeUncheckedVoucher(t, 800)

	for _, bad := range []string{"", "rejected", "bogus", "PENDING", "checked "} {
		// 直接写入异常状态，模拟脏数据/非法迁移值。
		if err := f.db.Model(&model.ExpenseVoucher{}).Where("id = ?", v.ID).
			Update("status", bad).Error; err != nil {
			t.Fatalf("inject status %q: %v", bad, err)
		}
		if _, err := f.svc.CheckVoucher(f.adminID, constants.RoleAdmin, v.ID); !errors.Is(err, ErrVoucherInvalidStatus) {
			t.Fatalf("abnormal status %q must be ErrVoucherInvalidStatus, got %v", bad, err)
		}
		// 被拒后状态原样保留，金额也不得计入已用。
		got, _ := repository.NewExpenseVoucherRepository(f.db).FindByID(v.ID)
		if got.Status != bad {
			t.Fatalf("status must remain %q after rejection, got %q", bad, got.Status)
		}
	}
	sum, _ := f.svc.GetSummary(f.projectID)
	if !almostEqual(sum.UsedAmount, 0) {
		t.Fatalf("abnormal voucher must never count as used, got %.2f", sum.UsedAmount)
	}

	// 恢复为 pending 后核验正常成功（证明白名单只放行 pending）。
	if err := f.db.Model(&model.ExpenseVoucher{}).Where("id = ?", v.ID).
		Update("status", constants.VoucherPending).Error; err != nil {
		t.Fatalf("reset status: %v", err)
	}
	checked, err := f.svc.CheckVoucher(f.adminID, constants.RoleAdmin, v.ID)
	if err != nil || checked.Status != constants.VoucherChecked {
		t.Fatalf("voucher should check normally once restored to pending, got %v %+v", err, checked)
	}
}

// 捐赠人公示只暴露已核验凭证；待核验凭证仅组织/平台可见。
func TestPublicFundsHidesUncheckedVouchers(t *testing.T) {
	f := newFundFixture(t)
	app := f.applyOK(t, 5000, "公示口径")
	if _, _, err := f.svc.Review(f.adminID, app.ID, ReviewInput{Approve: true}); err != nil {
		t.Fatalf("review: %v", err)
	}
	v1, err := f.svc.AddVoucher(f.orgUserID, app.ID, VoucherInput{Amount: 3000, Category: "物资", Usage: "已核验支出"})
	if err != nil {
		t.Fatalf("v1: %v", err)
	}
	if _, err := f.svc.AddVoucher(f.orgUserID, app.ID, VoucherInput{Amount: 1500, Category: "物流", Usage: "待核验支出"}); err != nil {
		t.Fatalf("v2: %v", err)
	}

	// 核验前：捐赠人什么凭证都看不到、已用为 0；组织内部两张都能看到。
	_, publicVouchers, sum, err := f.svc.PublicFunds(f.projectID)
	if err != nil || len(publicVouchers) != 0 || !almostEqual(sum.UsedAmount, 0) {
		t.Fatalf("before check public should see none/used=0, got %d %.2f %v", len(publicVouchers), sum.UsedAmount, err)
	}
	_, orgVouchers, err := f.svc.GetApplication(f.orgUserID, constants.RoleOrg, app.ID)
	if err != nil || len(orgVouchers) != 2 {
		t.Fatalf("org should see both vouchers, got %d %v", len(orgVouchers), err)
	}

	// 仅核验第一张：捐赠人只见 3000。
	if _, err := f.svc.CheckVoucher(f.adminID, constants.RoleAdmin, v1.ID); err != nil {
		t.Fatalf("check v1: %v", err)
	}
	_, publicVouchers, sum, _ = f.svc.PublicFunds(f.projectID)
	if len(publicVouchers) != 1 || !almostEqual(publicVouchers[0].Amount, 3000) || !almostEqual(sum.UsedAmount, 3000) {
		t.Fatalf("after checking v1 public used must be 3000, got vouchers=%d used=%.2f", len(publicVouchers), sum.UsedAmount)
	}
}
