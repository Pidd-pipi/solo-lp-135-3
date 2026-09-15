package service

import (
	"io"
	"log/slog"
	"testing"

	"github.com/givetrack/givetrack/internal/constants"
	"github.com/givetrack/givetrack/internal/model"
	"github.com/givetrack/givetrack/internal/repository"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// fundTestFixture 资金模块测试夹具。
type fundTestFixture struct {
	db             *gorm.DB
	svc            *DisbursementService
	orgUserID      uint
	otherOrgUserID uint
	adminID        uint
	orgID          uint
	projectID      uint
}

// newFundFixture 构造内存 SQLite 测试环境（纯 Go 驱动，无需 CGO）。
// 连接池限制为 1，既避免 SQLite 的 database is locked，也让并发事务确定地串行执行，
// 从而可验证“恰好一笔成功、其余被拒”的额度约束。
func newFundFixture(t *testing.T) *fundTestFixture {
	t.Helper()
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)

	if err := db.AutoMigrate(
		&model.User{}, &model.Organization{}, &model.Project{},
		&model.ProjectUpdate{}, &model.Donation{}, &model.AdminReview{},
		&model.VolunteerService{}, &model.DisbursementApplication{},
		&model.DisbursementOrder{}, &model.ExpenseVoucher{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	orgUser := &model.User{Username: "org1", Email: "org1@test.cn", Role: constants.RoleOrg, RealName: "组织一"}
	otherOrgUser := &model.User{Username: "org2", Email: "org2@test.cn", Role: constants.RoleOrg, RealName: "组织二"}
	admin := &model.User{Username: "admin", Email: "admin@test.cn", Role: constants.RoleAdmin}
	mustCreate(t, db, orgUser, otherOrgUser, admin)

	org := &model.Organization{UserID: orgUser.ID, Name: "组织一", Status: constants.OrgApproved}
	otherOrg := &model.Organization{UserID: otherOrgUser.ID, Name: "组织二", Status: constants.OrgApproved}
	mustCreate(t, db, org, otherOrg)

	project := &model.Project{
		OrganizationID: org.ID, Title: "测试助学项目", Category: "education",
		TargetAmount: 10000, CurrentAmount: 10000,
		Status: constants.ProjectApproved,
	}
	mustCreate(t, db, project)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewDisbursementService(
		db,
		repository.NewFundApplicationRepository(db),
		repository.NewDisbursementOrderRepository(db),
		repository.NewExpenseVoucherRepository(db),
		repository.NewProjectRepository(db),
		repository.NewOrganizationRepository(db),
		log,
	)
	return &fundTestFixture{
		db: db, svc: svc,
		orgUserID: orgUser.ID, otherOrgUserID: otherOrgUser.ID, adminID: admin.ID,
		orgID: org.ID, projectID: project.ID,
	}
}

func mustCreate(t *testing.T, db *gorm.DB, values ...interface{}) {
	t.Helper()
	for _, v := range values {
		if err := db.Create(v).Error; err != nil {
			t.Fatalf("seed create: %v", err)
		}
	}
}
