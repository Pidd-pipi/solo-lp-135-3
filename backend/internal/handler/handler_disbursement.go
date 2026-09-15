package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/givetrack/givetrack/internal/constants"
	"github.com/givetrack/givetrack/internal/repository"
	"github.com/givetrack/givetrack/internal/service"
	"github.com/givetrack/givetrack/internal/util"
)

// DisbursementHandler 资金拨付与用途凭证处理器。
type DisbursementHandler struct {
	disbSvc *service.DisbursementService
}

func NewDisbursementHandler(disbSvc *service.DisbursementService) *DisbursementHandler {
	return &DisbursementHandler{disbSvc: disbSvc}
}

// fail 资金模块错误映射：冲突类业务规则统一返回 409。
func (h *DisbursementHandler) fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		util.Fail(c, http.StatusNotFound, constants.CodeNotFound, err.Error())
	case errors.Is(err, service.ErrNotProjectOwner),
		errors.Is(err, service.ErrVoucherCheckForbidden):
		util.Fail(c, http.StatusForbidden, constants.CodeForbidden, err.Error())
	case errors.Is(err, service.ErrFundQuotaExceeded),
		errors.Is(err, service.ErrProjectSettled),
		errors.Is(err, service.ErrApplicationNotPending),
		errors.Is(err, service.ErrApplicationNotApproved),
		errors.Is(err, service.ErrVoucherExceedsOrder),
		errors.Is(err, service.ErrHasPendingApplication),
		errors.Is(err, service.ErrVoucherAlreadyChecked),
		errors.Is(err, service.ErrProjectNotFundable),
		errors.Is(err, repository.ErrConflict):
		util.Fail(c, http.StatusConflict, constants.CodeConflict, err.Error())
	case errors.Is(err, service.ErrVoucherCheckForbidden):
		util.Fail(c, http.StatusForbidden, constants.CodeForbidden, err.Error())
	default:
		util.FailError(c, err)
	}
}

func parseUintParam(c *gin.Context, key string) (uint, bool) {
	v, err := strconv.ParseUint(c.Param(key), 10, 64)
	if err != nil {
		util.Fail(c, http.StatusBadRequest, constants.CodeBadRequest, "invalid "+key)
		return 0, false
	}
	return uint(v), true
}

// Apply 组织提交用款申请。
func (h *DisbursementHandler) Apply(c *gin.Context) {
	projectID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var req service.ApplyInput
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Fail(c, http.StatusBadRequest, constants.CodeValidation, err.Error())
		return
	}
	app, err := h.disbSvc.Apply(c.GetUint("user_id"), projectID, req)
	if err != nil {
		h.fail(c, err)
		return
	}
	util.Created(c, gin.H{"message": "用款申请已提交，待平台审核", "application": app})
}

// Settle 组织结项项目。
func (h *DisbursementHandler) Settle(c *gin.Context) {
	projectID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	if err := h.disbSvc.SettleProject(c.GetUint("user_id"), projectID); err != nil {
		h.fail(c, err)
		return
	}
	util.OK(c, gin.H{"message": "项目已结项，冻结后续拨付"})
}

// OrgApplications 组织查看自己的用款申请。
func (h *DisbursementHandler) OrgApplications(c *gin.Context) {
	page, ps := util.NormalizePage(atoi(c.Query("page")), atoi(c.Query("page_size")))
	list, total, err := h.disbSvc.ListOrgApplications(c.GetUint("user_id"), c.Query("status"), page, ps)
	if err != nil {
		h.fail(c, err)
		return
	}
	util.OK(c, gin.H{"applications": list, "total": total, "page": page, "pageSize": ps})
}

// ApplicationDetail 组织/平台查看申请详情（含拨付单与凭证）。
func (h *DisbursementHandler) ApplicationDetail(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	app, vouchers, err := h.disbSvc.GetApplication(c.GetUint("user_id"), c.GetString("role"), id)
	if err != nil {
		h.fail(c, err)
		return
	}
	util.OK(c, gin.H{"application": app, "vouchers": vouchers})
}

// AddVoucher 组织回填支出凭证。
func (h *DisbursementHandler) AddVoucher(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var req service.VoucherInput
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Fail(c, http.StatusBadRequest, constants.CodeValidation, err.Error())
		return
	}
	v, err := h.disbSvc.AddVoucher(c.GetUint("user_id"), id, req)
	if err != nil {
		h.fail(c, err)
		return
	}
	util.Created(c, gin.H{"message": "支出凭证已回填", "voucher": v})
}

// PublicFunds 捐赠人公示：已审核拨付单与支出凭证。
func (h *DisbursementHandler) PublicFunds(c *gin.Context) {
	projectID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	orders, vouchers, summary, err := h.disbSvc.PublicFunds(projectID)
	if err != nil {
		h.fail(c, err)
		return
	}
	util.OK(c, gin.H{
		"disbursements": orders,
		"vouchers":      vouchers,
		"summary":       summary,
	})
}

// PendingApplications 平台待审核申请列表。
func (h *DisbursementHandler) PendingApplications(c *gin.Context) {
	page, ps := util.NormalizePage(atoi(c.Query("page")), atoi(c.Query("page_size")))
	list, total, err := h.disbSvc.ListPendingApplications(page, ps)
	if err != nil {
		h.fail(c, err)
		return
	}
	util.OK(c, gin.H{"applications": list, "total": total, "page": page, "pageSize": ps})
}

// AllApplications 平台全量申请追溯（可按项目/状态过滤）。
func (h *DisbursementHandler) AllApplications(c *gin.Context) {
	page, ps := util.NormalizePage(atoi(c.Query("page")), atoi(c.Query("page_size")))
	var projectID uint
	if pid := c.Query("project_id"); pid != "" {
		v, err := strconv.ParseUint(pid, 10, 64)
		if err != nil {
			util.Fail(c, http.StatusBadRequest, constants.CodeBadRequest, "invalid project_id")
			return
		}
		projectID = uint(v)
	}
	list, total, err := h.disbSvc.ListAllApplications(projectID, c.Query("status"), page, ps)
	if err != nil {
		h.fail(c, err)
		return
	}
	util.OK(c, gin.H{"applications": list, "total": total, "page": page, "pageSize": ps})
}

// Review 平台审核用款申请。
func (h *DisbursementHandler) Review(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var req service.ReviewInput
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Fail(c, http.StatusBadRequest, constants.CodeValidation, err.Error())
		return
	}
	app, order, err := h.disbSvc.Review(c.GetUint("user_id"), id, req)
	if err != nil {
		h.fail(c, err)
		return
	}
	util.OK(c, gin.H{
		"message":     "审核完成",
		"application": app,
		"order":       order,
	})
}

// CheckVoucher 平台核验支出凭证。
func (h *DisbursementHandler) CheckVoucher(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	v, err := h.disbSvc.CheckVoucher(c.GetUint("user_id"), c.GetString("role"), id)
	if err != nil {
		h.fail(c, err)
		return
	}
	util.OK(c, gin.H{"message": "凭证已核验", "voucher": v})
}
