package repository

import (
	"errors"
	"fmt"

	"github.com/givetrack/givetrack/internal/constants"
	"github.com/givetrack/givetrack/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// FundApplicationRepository 用款申请数据访问。
type FundApplicationRepository struct {
	db *gorm.DB
}

func NewFundApplicationRepository(db *gorm.DB) *FundApplicationRepository {
	return &FundApplicationRepository{db: db}
}

func (r *FundApplicationRepository) Create(a *model.DisbursementApplication) error {
	if err := r.db.Create(a).Error; err != nil {
		return fmt.Errorf("create fund application: %w", err)
	}
	return nil
}

// Update 更新申请（状态/审核意见）。
func (r *FundApplicationRepository) Update(a *model.DisbursementApplication) error {
	if err := r.db.Save(a).Error; err != nil {
		return fmt.Errorf("update fund application: %w", err)
	}
	return nil
}

func (r *FundApplicationRepository) FindByID(id uint) (*model.DisbursementApplication, error) {
	var a model.DisbursementApplication
	err := r.db.Preload("Project").Preload("Project.Organization").
		Preload("Order").First(&a, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find fund application by id: %w", err)
	}
	return &a, nil
}

// LockByID 在当前事务内对申请行加排他锁，串行化审核并发。
// MySQL 使用 FOR UPDATE；不支持的驱动（测试用 SQLite）退化为普通查询。
func (r *FundApplicationRepository) LockByID(id uint) (*model.DisbursementApplication, error) {
	var a model.DisbursementApplication
	q := r.db
	if r.db.Dialector.Name() == "mysql" {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := q.First(&a, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock fund application by id: %w", err)
	}
	return &a, nil
}

// OccupyingAmount 返回项目当前占用额度的申请金额合计（待审核 + 已通过）。
// 已驳回的申请不再占用额度；调用方须在持有项目行锁的事务内读取。
func (r *FundApplicationRepository) OccupyingAmount(projectID uint) (float64, error) {
	var total float64
	err := r.db.Model(&model.DisbursementApplication{}).
		Where("project_id = ? AND status IN ?", projectID,
			[]string{constants.FundApplyPending, constants.FundApplyApproved}).
		Select("COALESCE(SUM(amount),0)").Scan(&total).Error
	if err != nil {
		return 0, fmt.Errorf("sum occupying amount: %w", err)
	}
	return total, nil
}

// ListByProject 组织侧：查看项目下全部申请（含各状态），按时间倒序。
func (r *FundApplicationRepository) ListByProject(projectID uint) ([]model.DisbursementApplication, error) {
	var list []model.DisbursementApplication
	if err := r.db.Preload("Order").
		Where("project_id = ?", projectID).
		Order("created_at DESC").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list fund applications by project: %w", err)
	}
	return list, nil
}

// ListByOrg 组织侧：查看本组织全部用款申请。
func (r *FundApplicationRepository) ListByOrg(orgID uint, status string, page, pageSize int) ([]model.DisbursementApplication, int64, error) {
	var list []model.DisbursementApplication
	var total int64
	q := r.db.Model(&model.DisbursementApplication{}).
		Preload("Project").Preload("Order").Where("org_id = ?", orgID)
	if status != "" && status != "all" {
		q = q.Where("status = ?", status)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count fund applications by org: %w", err)
	}
	if err := q.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, fmt.Errorf("list fund applications by org: %w", err)
	}
	return list, total, nil
}

// ListPending 平台侧：待审核申请。
func (r *FundApplicationRepository) ListPending(page, pageSize int) ([]model.DisbursementApplication, int64, error) {
	var list []model.DisbursementApplication
	var total int64
	q := r.db.Model(&model.DisbursementApplication{}).
		Preload("Project").Preload("Project.Organization").
		Where("status = ?", constants.FundApplyPending)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count pending applications: %w", err)
	}
	if err := q.Order("created_at ASC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, fmt.Errorf("list pending applications: %w", err)
	}
	return list, total, nil
}

// ListAll 平台侧：全量申请追溯（可按项目/状态过滤）。
func (r *FundApplicationRepository) ListAll(projectID uint, status string, page, pageSize int) ([]model.DisbursementApplication, int64, error) {
	var list []model.DisbursementApplication
	var total int64
	q := r.db.Model(&model.DisbursementApplication{}).
		Preload("Project").Preload("Project.Organization").Preload("Order")
	if projectID > 0 {
		q = q.Where("project_id = ?", projectID)
	}
	if status != "" && status != "all" {
		q = q.Where("status = ?", status)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count all applications: %w", err)
	}
	if err := q.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, fmt.Errorf("list all applications: %w", err)
	}
	return list, total, nil
}

// DisbursementOrderRepository 拨付单数据访问。
type DisbursementOrderRepository struct {
	db *gorm.DB
}

func NewDisbursementOrderRepository(db *gorm.DB) *DisbursementOrderRepository {
	return &DisbursementOrderRepository{db: db}
}

// Create 创建拨付单。application_id/order_no 的唯一索引冲突会被转成 ErrConflict，
// 用于在并发审核时拒绝为同一申请重复生成拨付单。
func (r *DisbursementOrderRepository) Create(o *model.DisbursementOrder) error {
	if err := r.db.Create(o).Error; err != nil {
		if isDuplicateKeyErr(err) {
			return fmt.Errorf("create disbursement order: %w", ErrConflict)
		}
		return fmt.Errorf("create disbursement order: %w", err)
	}
	return nil
}

func (r *DisbursementOrderRepository) FindByApplicationID(applicationID uint) (*model.DisbursementOrder, error) {
	var o model.DisbursementOrder
	err := r.db.Where("application_id = ?", applicationID).First(&o).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find order by application: %w", err)
	}
	return &o, nil
}

func (r *DisbursementOrderRepository) FindByID(id uint) (*model.DisbursementOrder, error) {
	var o model.DisbursementOrder
	err := r.db.Preload("Application").Preload("Project").Preload("Vouchers").First(&o, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find order by id: %w", err)
	}
	return &o, nil
}

func (r *DisbursementOrderRepository) Update(o *model.DisbursementOrder) error {
	if err := r.db.Save(o).Error; err != nil {
		return fmt.Errorf("update disbursement order: %w", err)
	}
	return nil
}

// ListApprovedByProject 捐赠人公示：项目下已审核通过申请生成的拨付单。
func (r *DisbursementOrderRepository) ListApprovedByProject(projectID uint) ([]model.DisbursementOrder, error) {
	var list []model.DisbursementOrder
	if err := r.db.Where("project_id = ?", projectID).
		Order("created_at DESC").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list orders by project: %w", err)
	}
	return list, nil
}

// ExpenseVoucherRepository 支出凭证数据访问。
type ExpenseVoucherRepository struct {
	db *gorm.DB
}

func NewExpenseVoucherRepository(db *gorm.DB) *ExpenseVoucherRepository {
	return &ExpenseVoucherRepository{db: db}
}

func (r *ExpenseVoucherRepository) Create(v *model.ExpenseVoucher) error {
	if err := r.db.Create(v).Error; err != nil {
		if isDuplicateKeyErr(err) {
			return fmt.Errorf("create expense voucher: %w", ErrConflict)
		}
		return fmt.Errorf("create expense voucher: %w", err)
	}
	return nil
}

func (r *ExpenseVoucherRepository) FindByID(id uint) (*model.ExpenseVoucher, error) {
	var v model.ExpenseVoucher
	err := r.db.Preload("Order").Preload("Order.Application").First(&v, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find voucher by id: %w", err)
	}
	return &v, nil
}

// LockByID 在当前事务内对凭证行加排他锁，串行化核验并发。
// MySQL 使用 FOR UPDATE；不支持的驱动（测试用 SQLite）退化为普通查询。
func (r *ExpenseVoucherRepository) LockByID(id uint) (*model.ExpenseVoucher, error) {
	var v model.ExpenseVoucher
	q := r.db
	if r.db.Dialector.Name() == "mysql" {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := q.First(&v, id).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("lock voucher by id: %w", err)
	}
	return &v, nil
}

func (r *ExpenseVoucherRepository) Update(v *model.ExpenseVoucher) error {
	if err := r.db.Save(v).Error; err != nil {
		return fmt.Errorf("update expense voucher: %w", err)
	}
	return nil
}

// ListByProject 项目下全部支出凭证（含待核验），仅供所属组织/平台内部查看。
func (r *ExpenseVoucherRepository) ListByProject(projectID uint) ([]model.ExpenseVoucher, error) {
	var list []model.ExpenseVoucher
	if err := r.db.Where("project_id = ?", projectID).
		Order("spent_at DESC, created_at DESC").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list vouchers by project: %w", err)
	}
	return list, nil
}

// ListCheckedByProject 捐赠人公示：仅返回平台已核验通过的支出凭证。
func (r *ExpenseVoucherRepository) ListCheckedByProject(projectID uint) ([]model.ExpenseVoucher, error) {
	var list []model.ExpenseVoucher
	if err := r.db.Where("project_id = ? AND status = ?", projectID, constants.VoucherChecked).
		Order("spent_at DESC, created_at DESC").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list checked vouchers by project: %w", err)
	}
	return list, nil
}

// ListByOrder 拨付单下的凭证。
func (r *ExpenseVoucherRepository) ListByOrder(orderID uint) ([]model.ExpenseVoucher, error) {
	var list []model.ExpenseVoucher
	if err := r.db.Where("order_id = ?", orderID).
		Order("spent_at DESC, created_at DESC").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list vouchers by order: %w", err)
	}
	return list, nil
}

// VoucherTotal 项目已回填支出凭证金额合计（含待核验），供内部对账。
func (r *ExpenseVoucherRepository) VoucherTotal(projectID uint) (float64, error) {
	var total float64
	err := r.db.Model(&model.ExpenseVoucher{}).
		Where("project_id = ?", projectID).
		Select("COALESCE(SUM(amount),0)").Scan(&total).Error
	if err != nil {
		return 0, fmt.Errorf("sum voucher amount: %w", err)
	}
	return total, nil
}

// CheckedVoucherTotal 项目已核验支出凭证金额合计（捐赠人公示口径的"已用金额"）。
func (r *ExpenseVoucherRepository) CheckedVoucherTotal(projectID uint) (float64, error) {
	var total float64
	err := r.db.Model(&model.ExpenseVoucher{}).
		Where("project_id = ? AND status = ?", projectID, constants.VoucherChecked).
		Select("COALESCE(SUM(amount),0)").Scan(&total).Error
	if err != nil {
		return 0, fmt.Errorf("sum checked voucher amount: %w", err)
	}
	return total, nil
}
