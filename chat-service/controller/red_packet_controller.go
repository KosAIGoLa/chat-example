package controller

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"ws-ex/dto"
	"ws-ex/service"
	"ws-ex/validate"
)

// RedPacketController handles red packet HTTP APIs.
type RedPacketController struct {
	rp     *service.RedPacketService
	wallet *service.WalletService
}

func NewRedPacketController(rp *service.RedPacketService, wallet *service.WalletService) *RedPacketController {
	return &RedPacketController{rp: rp, wallet: wallet}
}

func (ctrl *RedPacketController) me(c *gin.Context) (uint, string, string) {
	raw, _ := c.Get("user_id")
	uid := raw.(uint)
	username, _ := c.Get("username")
	name, _ := username.(string)
	return uid, strconv.FormatUint(uint64(uid), 10), name
}

// GetWallet GET /api/wallet/me
func (ctrl *RedPacketController) GetWallet(c *gin.Context) {
	uid, _, _ := ctrl.me(c)
	bal, err := ctrl.wallet.GetBalance(uid)
	if err != nil {
		c.JSON(http.StatusNotFound, dto.APIResponseDTO{Code: 404, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.APIResponseDTO{
		Code: 200, Message: "success",
		Data: dto.WalletDTO{Balance: bal},
	})
}

// Create POST /api/red-packets
func (ctrl *RedPacketController) Create(c *gin.Context) {
	var req dto.CreateRedPacketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: validate.JSONBody(err).Error()})
		return
	}
	// Normalize + bound fields before service rules.
	req.Type = strings.ToLower(validate.CleanSingleLine(req.Type))
	switch req.Type {
	case "private", "group", "designated":
	default:
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "type must be private, group, or designated"})
		return
	}
	if req.PeerID != "" {
		peer, err := validate.PeerID(req.PeerID, true)
		if err != nil {
			c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
			return
		}
		req.PeerID = peer
	}
	if req.GroupID != "" {
		gid, err := validate.GroupID(req.GroupID, true)
		if err != nil {
			c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
			return
		}
		req.GroupID = gid
	}
	if err := validate.PositiveInt64(req.TotalAmount, "total_amount"); err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
		return
	}
	if req.TotalAmount > 1_000_000 {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "total_amount too large"})
		return
	}
	if req.TotalCount < 0 || req.TotalCount > 200 {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "total_count must be 0–200"})
		return
	}
	if len(req.TargetUserIDs) > 100 {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "too many target_user_ids"})
		return
	}
	cleanedTargets := make([]string, 0, len(req.TargetUserIDs))
	for _, t := range req.TargetUserIDs {
		id, err := validate.UserIDStr(t, true)
		if err != nil {
			c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "invalid target_user_ids: " + err.Error()})
			return
		}
		cleanedTargets = append(cleanedTargets, id)
	}
	req.TargetUserIDs = cleanedTargets
	greet, err := validate.Greeting(req.Greeting)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
		return
	}
	req.Greeting = greet

	uid, _, name := ctrl.me(c)
	packet, msg, err := ctrl.rp.Create(uid, name, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.APIResponseDTO{
		Code: 200, Message: "success",
		Data: gin.H{"packet": packet, "message": msg},
	})
}

// Claim POST /api/red-packets/:id/claim
func (ctrl *RedPacketController) Claim(c *gin.Context) {
	id, err := validate.ResourceID(c.Param("id"), true)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
		return
	}
	uid, _, name := ctrl.me(c)
	res, err := ctrl.rp.Claim(uid, name, id)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.APIResponseDTO{Code: 200, Message: "success", Data: res})
}

// Get GET /api/red-packets/:id
func (ctrl *RedPacketController) Get(c *gin.Context) {
	id, err := validate.ResourceID(c.Param("id"), true)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
		return
	}
	uid, _, _ := ctrl.me(c)
	res, err := ctrl.rp.Get(uid, id)
	if err != nil {
		c.JSON(http.StatusNotFound, dto.APIResponseDTO{Code: 404, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.APIResponseDTO{Code: 200, Message: "success", Data: res})
}
