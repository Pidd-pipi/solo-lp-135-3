package model

import "time"

// DisbursementApplication 用款申请。公益组织针对已通过的项目分批提交，
// 待审核（pending）与已通过（approved）的申请合计占用已筹资金额度。
type DisbursementApplication struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	ProjectID     uint       `gorm:"index;not null" json:"projectId"`
	OrgID         uint       `gorm:"index;not null" json:"orgId"`
	ApplicantID   uint       `gorm:"index;not null" json:"applicantId"`
	Amount        float64    `gorm:"type:decimal(14,2);not null" json:"amount"`
	Purpose       string     `gorm:"size:500;not null" json:"purpose"`
	BatchNo       string     `gorm:"size:64" json:"batchNo"`
	Status        string     `gorm:"size:20;index;not null;default:pending" json:"status"`
	ReviewerID    uint       `gorm:"index" json:"reviewerId"`
	ReviewComment string     `gorm:"size:500" json:"reviewComment"`
	ReviewedAt    *time.Time `gorm:"index" json:"reviewedAt"`
	CreatedAt     time.Time  `gorm:"index" json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`

	Project *Project           `gorm:"foreignKey:ProjectID" json:"project,omitempty"`
	Order   *DisbursementOrder `gorm:"foreignKey:ApplicationID" json:"order,omitempty"`
}

// DisbursementOrder 拨付单。审核通过后唯一生成，一个用款申请至多一张拨付单。
type DisbursementOrder struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	OrderNo       string     `gorm:"size:64;uniqueIndex;not null" json:"orderNo"`
	ApplicationID uint       `gorm:"uniqueIndex;not null" json:"applicationId"`
	ProjectID     uint       `gorm:"index;not null" json:"projectId"`
	OrgID         uint       `gorm:"index;not null" json:"orgId"`
	Amount        float64    `gorm:"type:decimal(14,2);not null" json:"amount"`
	Purpose       string     `gorm:"size:500" json:"purpose"`
	Status        string     `gorm:"size:20;index;not null;default:pending" json:"status"`
	PaidAt        *time.Time `gorm:"index" json:"paidAt"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`

	Application *DisbursementApplication `gorm:"foreignKey:ApplicationID" json:"application,omitempty"`
	Project     *Project                 `gorm:"foreignKey:ProjectID" json:"project,omitempty"`
	Vouchers    []ExpenseVoucher         `gorm:"foreignKey:OrderID" json:"vouchers,omitempty"`
}

// ExpenseVoucher 支出凭证。组织回填实际支出用途与凭证材料。
type ExpenseVoucher struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	OrderID       uint      `gorm:"index;not null" json:"orderId"`
	ApplicationID uint      `gorm:"index;not null" json:"applicationId"`
	ProjectID     uint      `gorm:"index;not null" json:"projectId"`
	Amount        float64   `gorm:"type:decimal(14,2);not null" json:"amount"`
	Category      string    `gorm:"size:64;not null" json:"category"`
	Usage         string    `gorm:"size:500;not null" json:"usage"`
	VoucherNo     string    `gorm:"size:64" json:"voucherNo"`
	InvoiceNo     string    `gorm:"size:64" json:"invoiceNo"`
	AttachmentURL string    `gorm:"size:255" json:"attachmentUrl"`
	ProgressNote  string    `gorm:"type:text" json:"progressNote"`
	SpentAt       time.Time `gorm:"index" json:"spentAt"`
	Status        string    `gorm:"size:20;index;not null;default:pending" json:"status"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`

	Order *DisbursementOrder `gorm:"foreignKey:OrderID" json:"order,omitempty"`
}
