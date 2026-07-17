package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"ws-ex/dto"
	"ws-ex/service"
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
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "invalid body"})
		return
	}
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
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "id required"})
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
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "id required"})
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
