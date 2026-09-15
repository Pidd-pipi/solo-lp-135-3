package constants

// 应用常量。
const (
	AppName         = "givetrack"
	APIVersion      = "v1"
	PageDefault     = 1
	PageSizeDefault = 10
	PageSizeMax     = 100
)

// 角色
const (
	RoleUser  = "user"
	RoleOrg   = "org"
	RoleAdmin = "admin"
)

// 项目状态
const (
	ProjectPending   = "pending"
	ProjectApproved  = "approved"
	ProjectRejected  = "rejected"
	ProjectCompleted = "completed"
)

// 用款申请状态
const (
	FundApplyPending  = "pending"  // 待审核（占用额度）
	FundApplyApproved = "approved" // 审核通过（已生成拨付单）
	FundApplyRejected = "rejected" // 审核驳回（释放额度）
)

// 拨付单状态
const (
	DisbursementPending = "pending" // 已生成，待拨付/回填
	DisbursementPaid    = "paid"    // 已拨付
)

// 支出凭证状态
const (
	VoucherPending = "pending" // 已回填，待核验
	VoucherChecked = "checked" // 平台核验通过
)

// 金额比较容差（decimal 以 float64 承载，避免浮点误差）。
const AmountEpsilon = 0.000001

// 组织审核状态
const (
	OrgPending  = "pending"
	OrgApproved = "approved"
	OrgRejected = "rejected"
)

// 支付状态
const (
	PaymentSuccess = "success"
	PaymentPending = "pending"
	PaymentFailed  = "failed"
)

// 项目分类
const (
	CategoryEducation   = "education"
	CategoryElderly     = "elderly"
	CategoryMedical     = "medical"
	CategoryDisaster    = "disaster"
	CategoryEnvironment = "environment"
	CategoryOther       = "other"
)

// 错误码
const (
	CodeOK              = 0
	CodeBadRequest      = 40000
	CodeUnauthorized    = 40100
	CodeForbidden       = 40300
	CodeNotFound        = 40400
	CodeConflict        = 40900
	CodeInternalError   = 50000
	CodeValidation      = 42200
	CodeTooManyRequests = 42900
)
