package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"ws-ex/dto"
	"ws-ex/service"
)

type AuthController struct {
	authSvc *service.AuthService
	hub     *service.Hub // optional; used to refresh display name when online
}

func NewAuthController(authSvc *service.AuthService) *AuthController {
	return &AuthController{authSvc: authSvc}
}

// SetHub links the WS hub so profile username changes propagate to presence.
func (ctrl *AuthController) SetHub(hub *service.Hub) {
	ctrl.hub = hub
}

func (ctrl *AuthController) Register(c *gin.Context) {
	var req dto.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{
			Code:    400,
			Message: err.Error(),
		})
		return
	}

	user, err := ctrl.authSvc.Register(req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusConflict, dto.APIResponseDTO{
			Code:    409,
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, dto.APIResponseDTO{
		Code:    201,
		Message: "user registered successfully",
		Data: dto.UserInfo{
			ID:       user.ID,
			Username: user.Username,
		},
	})
}

func (ctrl *AuthController) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{
			Code:    400,
			Message: err.Error(),
		})
		return
	}

	token, user, err := ctrl.authSvc.Login(req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, dto.APIResponseDTO{
			Code:    401,
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, dto.APIResponseDTO{
		Code:    200,
		Message: "login successful",
		Data: dto.LoginResponse{
			Token: token,
			User: dto.UserInfo{
				ID:       user.ID,
				Username: user.Username,
			},
		},
	})
}

// GetMe returns the authenticated user profile.
// GET /api/auth/me
func (ctrl *AuthController) GetMe(c *gin.Context) {
	userIDRaw, ok := c.Get("user_id")
	if !ok {
		c.JSON(http.StatusUnauthorized, dto.APIResponseDTO{Code: 401, Message: "unauthorized"})
		return
	}
	user, err := ctrl.authSvc.GetUser(userIDRaw.(uint))
	if err != nil {
		c.JSON(http.StatusNotFound, dto.APIResponseDTO{Code: 404, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.APIResponseDTO{
		Code:    200,
		Message: "success",
		Data: dto.UserInfo{
			ID:       user.ID,
			Username: user.Username,
		},
	})
}

// UpdateProfile updates the authenticated user's username / password.
// PUT /api/auth/profile
func (ctrl *AuthController) UpdateProfile(c *gin.Context) {
	userIDRaw, ok := c.Get("user_id")
	if !ok {
		c.JSON(http.StatusUnauthorized, dto.APIResponseDTO{Code: 401, Message: "unauthorized"})
		return
	}

	var req dto.UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
		return
	}

	uid := userIDRaw.(uint)
	token, user, err := ctrl.authSvc.UpdateProfile(
		uid,
		req.Username,
		req.Password,
		req.CurrentPassword,
	)
	if err != nil {
		status := http.StatusBadRequest
		if err.Error() == "username already exists" {
			status = http.StatusConflict
		}
		c.JSON(status, dto.APIResponseDTO{Code: status, Message: err.Error()})
		return
	}

	// Push updated display name to online presence / group lists.
	if ctrl.hub != nil {
		ctrl.hub.UpdateClientUsername(strconv.FormatUint(uint64(uid), 10), user.Username)
	}

	c.JSON(http.StatusOK, dto.APIResponseDTO{
		Code:    200,
		Message: "profile updated",
		Data: dto.LoginResponse{
			Token: token,
			User: dto.UserInfo{
				ID:       user.ID,
				Username: user.Username,
			},
		},
	})
}
