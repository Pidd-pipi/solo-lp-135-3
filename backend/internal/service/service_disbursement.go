package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/givetrack/givetrack/internal/constants"
	"github.com/givetrack/givetrack/internal/model"
	"github.com/givetrack/givetrack/internal/repository"
	"gorm.io/gorm"
)

// 业务哨兵错误：handler 据此映射 HTTP 状态码（冲突类返回 409）。
var (
	// ErrFundQuotaExceeded 待审与已批申请总额超过已筹资金。
	ErrFundQuotaExceeded = errors.New("quota exceeded: pending and approved requests exceed raised funds")
	// ErrProjectNotFundable 项目未通过审核，不可申请用款。
	ErrProjectNotFundable = errors.New("project not approved for disbursement")
	// ErrProjectSettled 项目已结项，禁止再申请或拨付。
	ErrProjectSettled = errors.New("project already settled")
	// ErrApplicationNotPending 申请不在待审核状态（重复审核）。
	ErrApplicationNotPending = errors.New("application is not pending: duplicate review")
	// ErrApplicationNotApproved 申请未通过，不可回填支出凭证。
	ErrApplicationNotApproved = errors.New("application not approved")
	// ErrVoucherExceedsOrder 本次支出凭证累计金额超过该拨付单金额。
	ErrVoucherExceedsOrder = errors.New("voucher amount exceeds disbursement order amount")
	// ErrHasPendingApplication 仍有未审核申请，不能结项。
	ErrHasPendingApplication = errors.New("project has pending applications and cannot settle")
	// ErrNotProjectOwner 非本项目所属组织。
	ErrNotProjectOwner = errors.New("forbidden: not the owner of this project")
)

// DisbursementService 资金拨付与用途凭证服务。
type DisbursementService struct {
	db          *gorm.DB
	appRepo     *repository.FundApplicationRepository
	orderRepo   *repository.DisbursementOrderRepository
	voucherRepo *repository.ExpenseVoucherRepository
	projectRepo *repository.ProjectRepository
	orgRepo     *repository.OrganizationRepository
	logger      *slog.Logger
	// keyedLocks 提供进程内按项目/申请维度的互斥，作为数据库行锁（跨进程）之外的纵深防御，
	// 保证读额度—校验—写入的临界区在单实例内不被并发穿插。
	locks *keyedLocks
}

// NewDisbursementService 构造拨付服务。
func NewDisbursementService(
	db *gorm.DB,
	appRepo *repository.FundApplicationRepository,
	orderRepo *repository.DisbursementOrderRepository,
	voucherRepo *repository.ExpenseVoucherRepository,
	projectRepo *repository.ProjectRepository,
	orgRepo *repository.OrganizationRepository,
	logger *slog.Logger,
) *DisbursementService {
	return &DisbursementService{
		db:          db,
		appRepo:     appRepo,
		orderRepo:   orderRepo,
		voucherRepo: voucherRepo,
		projectRepo: projectRepo,
		orgRepo:     orgRepo,
		logger:      logger,
		locks:       newKeyedLocks(),
	}
}

// ApplyInput 组织提交用款申请入参。
type ApplyInput struct {
	Amount  float64 `json:"amount" binding:"required,gt=0"`
	Purpose string  `json:"purpose" binding:"required,min=2,max=500"`
	BatchNo string  `json:"batchNo" binding:"omitempty,max=64"`
}

// Apply 组织针对已通过项目提交分批用款申请。
// 在事务内对项目行加排他锁，串行化并发提交，确保
// 待审 + 已批申请总额不超过已筹资金。
func (s *DisbursementService) Apply(userID, projectID uint, in ApplyInput) (*model.DisbursementApplication, error) {
	if projectID == 0 {
		return nil, fmt.Errorf("projectId is required")
	}
	if in.Amount <= 0 {
		return nil, fmt.Errorf("amount must be greater than zero")
	}
	if strings.TrimSpace(in.Purpose) == "" {
		return nil, fmt.Errorf("purpose is required")
	}
	unlock := s.locks.lock(projectKey(projectID))
	defer unlock()

	org, err := s.orgRepo.FindByUserID(userID)
	if err != nil {
		return nil, err
	}

	var app *model.DisbursementApplication
	err = s.db.Transaction(func(tx *gorm.DB) error {
		projectRepo := repository.NewProjectRepository(tx)
		appRepo := repository.NewFundApplicationRepository(tx)

		project, err := projectRepo.LockByID(projectID)
		if err != nil {
			return err
		}
		if project.OrganizationID != org.ID {
			return ErrNotProjectOwner
		}
		if project.SettledAt != nil {
			return ErrProjectSettled
		}
		if project.Status != constants.ProjectApproved && project.Status != constants.ProjectCompleted {
			return ErrProjectNotFundable
		}

		occupied, err := appRepo.OccupyingAmount(projectID)
		if err != nil {
			return err
		}
		if occupied+in.Amount > project.CurrentAmount+constants.AmountEpsilon {
			s.logger.Warn("fund application quota exceeded",
				"projectId", projectID, "raised", project.CurrentAmount,
				"occupied", occupied, "requested", in.Amount)
			return ErrFundQuotaExceeded
		}

		batchNo := strings.TrimSpace(in.BatchNo)
		if batchNo == "" {
			batchNo, err = s.nextBatchNo(tx, projectID)
			if err != nil {
				return err
			}
		}

		app = &model.DisbursementApplication{
			ProjectID:   projectID,
			OrgID:       org.ID,
			ApplicantID: userID,
			Amount:      in.Amount,
			Purpose:     strings.TrimSpace(in.Purpose),
			BatchNo:     batchNo,
			Status:      constants.FundApplyPending,
		}
		if err := appRepo.Create(app); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.logger.Info("fund application submitted", "applicationId", app.ID,
		"projectId", projectID, "amount", in.Amount)
	return app, nil
}

// nextBatchNo 生成项目内递增的批次号，如 B-P3-01。
func (s *DisbursementService) nextBatchNo(tx *gorm.DB, projectID uint) (string, error) {
	var count int64
	if err := tx.Model(&model.DisbursementApplication{}).
		Where("project_id = ?", projectID).Count(&count).Error; err != nil {
		return "", fmt.Errorf("count applications for batch: %w", err)
	}
	return fmt.Sprintf("B-P%d-%02d", projectID, count+1), nil
}

// ReviewInput 平台审核入参。
type ReviewInput struct {
	Approve bool   `json:"approve"`
	Comment string `json:"comment" binding:"omitempty,max=500"`
}

// Review 平台审核用款申请。通过则在同一事务内生成唯一拨付单；驳回则释放额度。
// 对申请行加锁 + application_id 唯一索引，重复审核与并发审核都会被拒绝。
func (s *DisbursementService) Review(reviewerID, applicationID uint, in ReviewInput) (*model.DisbursementApplication, *model.DisbursementOrder, error) {
	var (
		app   *model.DisbursementApplication
		order *model.DisbursementOrder
	)
	unlock := s.locks.lock(applicationKey(applicationID))
	defer unlock()
	err := s.db.Transaction(func(tx *gorm.DB) error {
		appRepo := repository.NewFundApplicationRepository(tx)
		orderRepo := repository.NewDisbursementOrderRepository(tx)
		projectRepo := repository.NewProjectRepository(tx)

		var err error
		app, err = appRepo.LockByID(applicationID)
		if err != nil {
			return err
		}
		if app.Status != constants.FundApplyPending {
			return fmt.Errorf("%w: current status=%s", ErrApplicationNotPending, app.Status)
		}

		now := time.Now()
		app.ReviewerID = reviewerID
		app.ReviewComment = strings.TrimSpace(in.Comment)
		app.ReviewedAt = &now

		if !in.Approve {
			app.Status = constants.FundApplyRejected
			if err := appRepo.Update(app); err != nil {
				return err
			}
			return nil
		}

		// 通过前复核：结项项目禁止拨款；额度以当前已筹为准（已筹只增不减，
		// 已占用额度在申请创建时校验过，这里再校验一次以防越界数据）。
		project, err := projectRepo.LockByID(app.ProjectID)
		if err != nil {
			return err
		}
		if project.SettledAt != nil {
			return ErrProjectSettled
		}
		occupied, err := appRepo.OccupyingAmount(app.ProjectID)
		if err != nil {
			return err
		}
		// occupied 已包含本笔待审金额；通过后它仍占用（变为已批），故不增加占用。
		if occupied > project.CurrentAmount+constants.AmountEpsilon {
			return ErrFundQuotaExceeded
		}

		app.Status = constants.FundApplyApproved
		if err := appRepo.Update(app); err != nil {
			return err
		}

		// 审核通过即完成拨付：生成唯一拨付单并标记已拨付、记录拨付时间。
		orderNo, err := newOrderNo()
		if err != nil {
			return err
		}
		paidAt := time.Now()
		order = &model.DisbursementOrder{
			OrderNo:       orderNo,
			ApplicationID: app.ID,
			ProjectID:     app.ProjectID,
			OrgID:         app.OrgID,
			Amount:        app.Amount,
			Purpose:       app.Purpose,
			Status:        constants.DisbursementPaid,
			PaidAt:        &paidAt,
		}
		// application_id 唯一索引兜底并发：第二个并发审核在此冲突回滚。
		if err := orderRepo.Create(order); err != nil {
			if errors.Is(err, repository.ErrConflict) {
				return fmt.Errorf("%w: order already exists", ErrApplicationNotPending)
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	s.logger.Info("fund application reviewed", "applicationId", applicationID,
		"approved", in.Approve, "orderNo", orderNoOf(order))
	return app, order, nil
}

func orderNoOf(o *model.DisbursementOrder) string {
	if o == nil {
		return ""
	}
	return o.OrderNo
}

// VoucherInput 组织回填支出凭证入参。
type VoucherInput struct {
	Amount        float64 `json:"amount" binding:"required,gt=0"`
	Category      string  `json:"category" binding:"required,min=1,max=64"`
	Usage         string  `json:"usage" binding:"required,min=2,max=500"`
	InvoiceNo     string  `json:"invoiceNo" binding:"omitempty,max=64"`
	AttachmentURL string  `json:"attachmentUrl" binding:"omitempty,max=255"`
	ProgressNote  string  `json:"progressNote" binding:"omitempty,max=2000"`
	SpentAt       string  `json:"spentAt" binding:"omitempty,datetime=2006-01-02"`
}

// AddVoucher 组织针对已审核拨付单回填支出凭证与执行进展。
// 同一拨付单累计支出不得超过拨付金额。
func (s *DisbursementService) AddVoucher(userID, applicationID uint, in VoucherInput) (*model.ExpenseVoucher, error) {
	if applicationID == 0 {
		return nil, fmt.Errorf("applicationId is required")
	}
	if in.Amount <= 0 {
		return nil, fmt.Errorf("amount must be greater than zero")
	}

	org, err := s.orgRepo.FindByUserID(userID)
	if err != nil {
		return nil, err
	}
	unlock := s.locks.lock(applicationKey(applicationID))
	defer unlock()

	var voucher *model.ExpenseVoucher
	err = s.db.Transaction(func(tx *gorm.DB) error {
		appRepo := repository.NewFundApplicationRepository(tx)
		orderRepo := repository.NewDisbursementOrderRepository(tx)
		voucherRepo := repository.NewExpenseVoucherRepository(tx)

		app, err := appRepo.LockByID(applicationID)
		if err != nil {
			return err
		}
		if app.OrgID != org.ID {
			return ErrNotProjectOwner
		}
		if app.Status != constants.FundApplyApproved {
			return ErrApplicationNotApproved
		}
		order, err := orderRepo.FindByApplicationID(applicationID)
		if err != nil {
			return err
		}

		existing, err := voucherRepo.ListByOrder(order.ID)
		if err != nil {
			return err
		}
		var used float64
		for i := range existing {
			used += existing[i].Amount
		}
		if used+in.Amount > order.Amount+constants.AmountEpsilon {
			return fmt.Errorf("%w: order=%s used=%.2f adding=%.2f limit=%.2f",
				ErrVoucherExceedsOrder, order.OrderNo, used, in.Amount, order.Amount)
		}

		spentAt := time.Now()
		if in.SpentAt != "" {
			if t, perr := time.Parse("2006-01-02", in.SpentAt); perr == nil {
				spentAt = t
			}
		}
		voucherNo, err := newOrderNo()
		if err != nil {
			return err
		}
		voucherNo = "EXP" + strings.TrimPrefix(voucherNo, "DF")

		voucher = &model.ExpenseVoucher{
			OrderID:       order.ID,
			ApplicationID: applicationID,
			ProjectID:     app.ProjectID,
			Amount:        in.Amount,
			Category:      strings.TrimSpace(in.Category),
			Usage:         strings.TrimSpace(in.Usage),
			VoucherNo:     voucherNo,
			InvoiceNo:     strings.TrimSpace(in.InvoiceNo),
			AttachmentURL: strings.TrimSpace(in.AttachmentURL),
			ProgressNote:  strings.TrimSpace(in.ProgressNote),
			SpentAt:       spentAt,
			Status:        constants.VoucherPending,
		}
		if err := voucherRepo.Create(voucher); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.logger.Info("expense voucher added", "voucherId", voucher.ID,
		"applicationId", applicationID, "amount", in.Amount)
	return voucher, nil
}

// CheckVoucher 平台核验支出凭证。
func (s *DisbursementService) CheckVoucher(reviewerID, voucherID uint) (*model.ExpenseVoucher, error) {
	v, err := s.voucherRepo.FindByID(voucherID)
	if err != nil {
		return nil, err
	}
	v.Status = constants.VoucherChecked
	if err := s.voucherRepo.Update(v); err != nil {
		return nil, err
	}
	s.logger.Info("expense voucher checked", "voucherId", voucherID, "reviewerId", reviewerID)
	return v, nil
}

// SettleProject 组织对项目结项。结项后禁止再申请或拨付。
// 仍存在待审核用款申请时拒绝结项。
func (s *DisbursementService) SettleProject(userID, projectID uint) error {
	org, err := s.orgRepo.FindByUserID(userID)
	if err != nil {
		return err
	}
	unlock := s.locks.lock(projectKey(projectID))
	defer unlock()
	return s.db.Transaction(func(tx *gorm.DB) error {
		projectRepo := repository.NewProjectRepository(tx)
		appRepo := repository.NewFundApplicationRepository(tx)

		project, err := projectRepo.LockByID(projectID)
		if err != nil {
			return err
		}
		if project.OrganizationID != org.ID {
			return ErrNotProjectOwner
		}
		if project.SettledAt != nil {
			return ErrProjectSettled
		}

		pending, _, err := appRepo.ListAll(projectID, constants.FundApplyPending, 1, 1)
		if err != nil {
			return err
		}
		if len(pending) > 0 {
			return ErrHasPendingApplication
		}

		now := time.Now()
		project.SettledAt = &now
		project.Status = constants.ProjectCompleted
		return projectRepo.Update(project)
	})
}

// FundSummary 按项目汇总已筹、已拨、已用与剩余额度。
type FundSummary struct {
	ProjectID       uint    `json:"projectId"`
	ProjectTitle    string  `json:"projectTitle"`
	RaisedAmount    float64 `json:"raisedAmount"`
	OccupiedAmount  float64 `json:"occupiedAmount"`  // 待审 + 已批占用
	DisbursedAmount float64 `json:"disbursedAmount"` // 已生成拨付单金额
	UsedAmount      float64 `json:"usedAmount"`      // 已回填支出凭证金额
	PendingAmount   float64 `json:"pendingAmount"`   // 待审核申请金额
	RemainingAmount float64 `json:"remainingAmount"` // 已筹 - 占用
}

// GetSummary 项目资金汇总。
func (s *DisbursementService) GetSummary(projectID uint) (*FundSummary, error) {
	project, err := s.projectRepo.FindByID(projectID)
	if err != nil {
		return nil, err
	}
	occupied, err := s.appRepo.OccupyingAmount(projectID)
	if err != nil {
		return nil, err
	}

	var pendingTotal float64
	if err := s.db.Model(&model.DisbursementApplication{}).
		Where("project_id = ? AND status = ?", projectID, constants.FundApplyPending).
		Select("COALESCE(SUM(amount),0)").Scan(&pendingTotal).Error; err != nil {
		return nil, fmt.Errorf("sum pending amount: %w", err)
	}

	var disbursed float64
	if err := s.db.Model(&model.DisbursementOrder{}).
		Where("project_id = ?", projectID).
		Select("COALESCE(SUM(amount),0)").Scan(&disbursed).Error; err != nil {
		return nil, fmt.Errorf("sum disbursed amount: %w", err)
	}

	used, err := s.voucherRepo.VoucherTotal(projectID)
	if err != nil {
		return nil, err
	}

	return &FundSummary{
		ProjectID:       projectID,
		ProjectTitle:    project.Title,
		RaisedAmount:    project.CurrentAmount,
		OccupiedAmount:  occupied,
		DisbursedAmount: disbursed,
		UsedAmount:      used,
		PendingAmount:   pendingTotal,
		RemainingAmount: project.CurrentAmount - occupied,
	}, nil
}

// PublicFunds 捐赠人公示视图：项目下已审核通过的拨付单与全部支出凭证。
func (s *DisbursementService) PublicFunds(projectID uint) (orders []model.DisbursementOrder, vouchers []model.ExpenseVoucher, summary *FundSummary, err error) {
	if _, err = s.projectRepo.FindByID(projectID); err != nil {
		return nil, nil, nil, err
	}
	orders, err = s.orderRepo.ListApprovedByProject(projectID)
	if err != nil {
		return nil, nil, nil, err
	}
	vouchers, err = s.voucherRepo.ListByProject(projectID)
	if err != nil {
		return nil, nil, nil, err
	}
	summary, err = s.GetSummary(projectID)
	if err != nil {
		return nil, nil, nil, err
	}
	return orders, vouchers, summary, nil
}

// ListOrgApplications 组织查看自己的用款申请。
func (s *DisbursementService) ListOrgApplications(userID uint, status string, page, pageSize int) ([]model.DisbursementApplication, int64, error) {
	org, err := s.orgRepo.FindByUserID(userID)
	if err != nil {
		return nil, 0, err
	}
	return s.appRepo.ListByOrg(org.ID, status, page, pageSize)
}

// GetApplication 组织/平台查看申请详情（含拨付单与凭证）。
func (s *DisbursementService) GetApplication(userID uint, role string, applicationID uint) (*model.DisbursementApplication, []model.ExpenseVoucher, error) {
	app, err := s.appRepo.FindByID(applicationID)
	if err != nil {
		return nil, nil, err
	}
	if role != constants.RoleAdmin {
		org, err := s.orgRepo.FindByUserID(userID)
		if err != nil {
			return nil, nil, err
		}
		if app.OrgID != org.ID {
			return nil, nil, ErrNotProjectOwner
		}
	}
	var vouchers []model.ExpenseVoucher
	if app.Order != nil {
		vouchers, err = s.voucherRepo.ListByOrder(app.Order.ID)
		if err != nil {
			return nil, nil, err
		}
	}
	return app, vouchers, nil
}

// ListPendingApplications 平台查看待审核申请。
func (s *DisbursementService) ListPendingApplications(page, pageSize int) ([]model.DisbursementApplication, int64, error) {
	return s.appRepo.ListPending(page, pageSize)
}

// ListAllApplications 平台追溯全量申请（可按项目/状态过滤）。
func (s *DisbursementService) ListAllApplications(projectID uint, status string, page, pageSize int) ([]model.DisbursementApplication, int64, error) {
	return s.appRepo.ListAll(projectID, status, page, pageSize)
}

func projectKey(projectID uint) string {
	return fmt.Sprintf("project:%d", projectID)
}

func applicationKey(applicationID uint) string {
	return fmt.Sprintf("application:%d", applicationID)
}

// newOrderNo 生成唯一单号：DF + 12 位十六进制随机串。
func newOrderNo() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate order number: %w", err)
	}
	return "DF" + hex.EncodeToString(b), nil
}
