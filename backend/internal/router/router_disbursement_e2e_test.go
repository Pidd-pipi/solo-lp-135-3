package router

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/givetrack/givetrack/internal/config"
	"github.com/givetrack/givetrack/internal/constants"
	"github.com/givetrack/givetrack/internal/model"
	"github.com/givetrack/givetrack/internal/repository"
	"github.com/givetrack/givetrack/internal/service"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type e2eEnv struct {
	t         *testing.T
	r         http.Handler
	db        *gorm.DB
	authSvc   *service.AuthService
	orgID     uint
	projectID uint
	orgTok    string
	adminTok  string
	donorTok  string
}

func setupE2E(t *testing.T) *e2eEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(
		&model.User{}, &model.Organization{}, &model.Project{}, &model.ProjectUpdate{},
		&model.Donation{}, &model.AdminReview{}, &model.VolunteerService{},
		&model.DisbursementApplication{}, &model.DisbursementOrder{}, &model.ExpenseVoucher{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	orgUser := &model.User{Username: "org", Email: "org@e.cn", PasswordHash: "x", Role: constants.RoleOrg}
	admin := &model.User{Username: "admin", Email: "admin@e.cn", PasswordHash: "x", Role: constants.RoleAdmin}
	donor := &model.User{Username: "donor", Email: "donor@e.cn", PasswordHash: "x", Role: constants.RoleUser}
	if err := db.Create(orgUser).Error; err != nil {
		t.Fatalf("create users: %v", err)
	}
	db.Create(admin)
	db.Create(donor)
	org := &model.Organization{UserID: orgUser.ID, Name: "组织", Status: constants.OrgApproved}
	db.Create(org)
	project := &model.Project{OrganizationID: org.ID, Title: "E2E 项目", Category: "education",
		TargetAmount: 10000, CurrentAmount: 10000, Status: constants.ProjectApproved}
	db.Create(project)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	userRepo := repository.NewUserRepository(db)
	orgRepo := repository.NewOrganizationRepository(db)
	projectRepo := repository.NewProjectRepository(db)
	updateRepo := repository.NewProjectUpdateRepository(db)
	donationRepo := repository.NewDonationRepository(db)
	reviewRepo := repository.NewAdminReviewRepository(db)
	appRepo := repository.NewFundApplicationRepository(db)
	orderRepo := repository.NewDisbursementOrderRepository(db)
	voucherRepo := repository.NewExpenseVoucherRepository(db)

	authSvc := service.NewAuthService(userRepo, orgRepo, "test-secret-at-least-32-characters-long", 24, log)
	projectSvc := service.NewProjectService(projectRepo, updateRepo, orgRepo, donationRepo, log)
	donationSvc := service.NewDonationService(db, donationRepo, projectRepo, userRepo, log)
	rankingSvc := service.NewRankingService(userRepo, log)
	adminSvc := service.NewAdminService(projectRepo, orgRepo, reviewRepo, log)
	disbSvc := service.NewDisbursementService(db, appRepo, orderRepo, voucherRepo, projectRepo, orgRepo, log)

	cfg := &config.Config{CORSAllowedOrigins: "*", AuthRateLimit: 100, AuthRateWindowSecs: 60}
	r := Setup(db, authSvc, projectSvc, donationSvc, rankingSvc, adminSvc, disbSvc, cfg, log)

	orgTok, _ := authSvc.GenerateToken(orgUser)
	adminTok, _ := authSvc.GenerateToken(admin)
	donorTok, _ := authSvc.GenerateToken(donor)

	return &e2eEnv{
		t: t, r: r, db: db, authSvc: authSvc, orgID: org.ID, projectID: project.ID,
		orgTok: orgTok, adminTok: adminTok, donorTok: donorTok,
	}
}

type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func (e *e2eEnv) do(method, path, token string, body interface{}) (int, envelope) {
	e.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	e.r.ServeHTTP(w, req)

	var env envelope
	if w.Body.Len() > 0 {
		_ = json.Unmarshal(w.Body.Bytes(), &env)
	}
	return w.Code, env
}

func dataMap(t *testing.T, env envelope) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(env.Data, &m); err != nil {
		t.Fatalf("decode data %s: %v", string(env.Data), err)
	}
	return m
}

// TestDisbursementHTTPFlow 通过真实路由走通：申请→审核→拨付→回填→公示，
// 并验证超额、重复审核、结项后再拨、越权等场景返回正确的 HTTP 状态码。
func TestDisbursementHTTPFlow(t *testing.T) {
	e := setupE2E(t)
	pid := e.projectID

	// 未登录访问被拒。
	if code, _ := e.do(http.MethodPost, "/api/v1/projects/1/disbursements", "", map[string]interface{}{"amount": 1, "purpose": "x"}); code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated apply should be 401, got %d", code)
	}
	// 普通捐赠人无权申请（RBAC 403）。
	if code, _ := e.do(http.MethodPost, pathApply(pid), e.donorTok, map[string]interface{}{"amount": 1, "purpose": "x"}); code != http.StatusForbidden {
		t.Fatalf("donor apply should be 403, got %d", code)
	}

	// 1. 组织提交 6000 用款申请 → 201。
	code, env := e.do(http.MethodPost, pathApply(pid), e.orgTok, map[string]interface{}{"amount": 6000, "purpose": "采购课外读物"})
	if code != http.StatusCreated || env.Code != constants.CodeOK {
		t.Fatalf("apply expected 201, got %d env=%+v", code, env)
	}
	app := dataMap(t, env)["application"].(map[string]interface{})
	appID := uint(app["id"].(float64))

	// 2. 超额申请 → 409（6000 待审 + 5000 > 10000）。
	if code, env := e.do(http.MethodPost, pathApply(pid), e.orgTok, map[string]interface{}{"amount": 5000, "purpose": "超额"}); code != http.StatusConflict || env.Code != constants.CodeConflict {
		t.Fatalf("over-quota apply should be 409/40900, got %d/%d %s", code, env.Code, env.Message)
	}

	// 3. 捐赠人查看公示：审核前尚无拨付单，但汇总可见待审占用。
	code, env = e.do(http.MethodGet, pathFunds(pid), e.donorTok, nil)
	if code != http.StatusOK {
		t.Fatalf("public funds get: %d", code)
	}
	pub := dataMap(t, env)
	if len(pub["disbursements"].([]interface{})) != 0 {
		t.Fatalf("no approved disbursement should be public before review")
	}
	summary := pub["summary"].(map[string]interface{})
	if summary["occupiedAmount"].(float64) != 6000 || summary["remainingAmount"].(float64) != 4000 {
		t.Fatalf("unexpected pre-review summary: %v", summary)
	}

	// 4. 管理员待审列表包含该申请。
	code, env = e.do(http.MethodGet, "/api/v1/admin/disbursements/pending", e.adminTok, nil)
	if code != http.StatusOK || len(dataMap(t, env)["applications"].([]interface{})) != 1 {
		t.Fatalf("pending list should contain 1, got %d", code)
	}

	// 5. 审核通过 → 生成唯一拨付单。
	code, env = e.do(http.MethodPost, pathReview(appID), e.adminTok, map[string]interface{}{"approve": true, "comment": "同意拨付"})
	if code != http.StatusOK {
		t.Fatalf("review approve expected 200, got %d %s", code, env.Message)
	}
	reviewData := dataMap(t, env)
	order := reviewData["order"].(map[string]interface{})
	if order["orderNo"].(string) == "" {
		t.Fatalf("orderNo required")
	}

	// 6. 重复审核 → 409。
	if code, env := e.do(http.MethodPost, pathReview(appID), e.adminTok, map[string]interface{}{"approve": true}); code != http.StatusConflict {
		t.Fatalf("duplicate review should be 409, got %d %s", code, env.Message)
	}

	// 7. 回填凭证：4000 通过，再 2500 超额拒绝，补 2000 恰好满额通过。
	pathVoucher := "/api/v1/disbursements/applications/" + itoa(appID) + "/vouchers"
	code, env = e.do(http.MethodPost, pathVoucher, e.orgTok, map[string]interface{}{
		"amount": 4000, "category": "物资", "usage": "课外读物 3000 册", "invoiceNo": "INV-1",
	})
	if code != http.StatusCreated {
		t.Fatalf("voucher 4000 expected 201, got %d %s", code, env.Message)
	}
	voucher1ID := uint(dataMap(t, env)["voucher"].(map[string]interface{})["id"].(float64))
	if code, _ := e.do(http.MethodPost, pathVoucher, e.orgTok, map[string]interface{}{
		"amount": 2500, "category": "物流", "usage": "超额配送",
	}); code != http.StatusConflict {
		t.Fatalf("over-limit voucher should be 409, got %d", code)
	}
	code, env = e.do(http.MethodPost, pathVoucher, e.orgTok, map[string]interface{}{
		"amount": 2000, "category": "物流", "usage": "书架配送安装", "progressNote": "已完成首批配送",
	})
	if code != http.StatusCreated {
		t.Fatalf("voucher 2000 expected 201, got %d %s", code, env.Message)
	}
	voucher2ID := uint(dataMap(t, env)["voucher"].(map[string]interface{})["id"].(float64))

	// 8. 未核验前：捐赠人看不到凭证、已用金额为 0。
	_, env = e.do(http.MethodGet, pathFunds(pid), e.donorTok, nil)
	pub = dataMap(t, env)
	if len(pub["vouchers"].([]interface{})) != 0 {
		t.Fatalf("unchecked vouchers must not be public, got %d", len(pub["vouchers"].([]interface{})))
	}
	if pub["summary"].(map[string]interface{})["usedAmount"].(float64) != 0 {
		t.Fatalf("used amount must be 0 before checking: %v", pub["summary"])
	}

	// 9. 核验权限：组织/捐赠人均无权（403）。
	pathCheck := func(vid uint) string { return "/api/v1/admin/disbursements/vouchers/" + itoa(vid) + "/check" }
	if code, _ := e.do(http.MethodPost, pathCheck(voucher1ID), e.orgTok, nil); code != http.StatusForbidden {
		t.Fatalf("org check should be 403, got %d", code)
	}
	if code, _ := e.do(http.MethodPost, pathCheck(voucher1ID), e.donorTok, nil); code != http.StatusForbidden {
		t.Fatalf("donor check should be 403, got %d", code)
	}

	// 10. 管理员核验第一张：成功；重复核验 → 409。
	if code, env := e.do(http.MethodPost, pathCheck(voucher1ID), e.adminTok, nil); code != http.StatusOK {
		t.Fatalf("admin check expected 200, got %d %s", code, env.Message)
	}
	if code, env := e.do(http.MethodPost, pathCheck(voucher1ID), e.adminTok, nil); code != http.StatusConflict {
		t.Fatalf("duplicate check should be 409, got %d %s", code, env.Message)
	}
	// 管理员核验第二张。
	if code, _ := e.do(http.MethodPost, pathCheck(voucher2ID), e.adminTok, nil); code != http.StatusOK {
		t.Fatalf("check voucher2 expected 200, got %d", code)
	}
	if code, _ := e.do(http.MethodPost, pathCheck(voucher2ID), e.adminTok, nil); code != http.StatusConflict {
		t.Fatalf("repeat check voucher2 should be 409, got %d", code)
	}

	// 11. 核验后捐赠人公示：1 张拨付单、2 张凭证、已用 6000。
	_, env = e.do(http.MethodGet, pathFunds(pid), e.donorTok, nil)
	pub = dataMap(t, env)
	if len(pub["disbursements"].([]interface{})) != 1 {
		t.Fatalf("public should show 1 disbursement")
	}
	if len(pub["vouchers"].([]interface{})) != 2 {
		t.Fatalf("public should show 2 checked vouchers")
	}
	summary = pub["summary"].(map[string]interface{})
	if summary["disbursedAmount"].(float64) != 6000 || summary["usedAmount"].(float64) != 6000 ||
		summary["remainingAmount"].(float64) != 4000 {
		t.Fatalf("unexpected final summary: %v", summary)
	}

	// 12. 结项：无待审申请，成功；结项后再申请 → 409。
	if code, env := e.do(http.MethodPost, "/api/v1/projects/"+itoa(pid)+"/settle", e.orgTok, nil); code != http.StatusOK {
		t.Fatalf("settle expected 200, got %d %s", code, env.Message)
	}
	if code, env := e.do(http.MethodPost, pathApply(pid), e.orgTok, map[string]interface{}{"amount": 100, "purpose": "结项后"}); code != http.StatusConflict {
		t.Fatalf("apply after settle should be 409, got %d %s", code, env.Message)
	}
}

func pathApply(projectID uint) string {
	return "/api/v1/projects/" + itoa(projectID) + "/disbursements"
}

func pathFunds(projectID uint) string {
	return "/api/v1/projects/" + itoa(projectID) + "/funds"
}

func pathReview(appID uint) string {
	return "/api/v1/admin/disbursements/applications/" + itoa(appID) + "/review"
}

func itoa(v uint) string {
	return strconv.FormatUint(uint64(v), 10)
}
