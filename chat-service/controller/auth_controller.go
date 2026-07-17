package controller

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"ws-ex/dto"
	"ws-ex/model"
	"ws-ex/service"
)

type AuthController struct {
	authSvc *service.AuthService
	media   *service.MediaService
	hub     *service.Hub // optional; used to refresh display name when online
}

func NewAuthController(authSvc *service.AuthService) *AuthController {
	return &AuthController{authSvc: authSvc}
}

// SetMedia enables avatar upload storage.
func (ctrl *AuthController) SetMedia(m *service.MediaService) {
	ctrl.media = m
}

func userInfoDTO(u *model.User) dto.UserInfo {
	if u == nil {
		return dto.UserInfo{}
	}
	return dto.UserInfo{
		ID:        u.ID,
		Username:  u.Username,
		Balance:   u.Balance,
		Avatar:    u.Avatar,
		AvatarRev: u.AvatarRev,
	}
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
		Data:    userInfoDTO(user),
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
			User:  userInfoDTO(user),
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
		Data:    userInfoDTO(user),
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
			User:  userInfoDTO(user),
		},
	})
}

// UploadAvatar POST /api/avatar  multipart field "file"
func (ctrl *AuthController) UploadAvatar(c *gin.Context) {
	if ctrl.media == nil {
		c.JSON(http.StatusServiceUnavailable, dto.APIResponseDTO{Code: 503, Message: "media not configured"})
		return
	}
	userIDRaw, ok := c.Get("user_id")
	if !ok {
		c.JSON(http.StatusUnauthorized, dto.APIResponseDTO{Code: 401, Message: "unauthorized"})
		return
	}
	uid := userIDRaw.(uint)

	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "file is required"})
		return
	}
	if fh.Size > service.MaxAvatarBytes {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "file too large (max 2MB)"})
		return
	}
	src, err := fh.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.APIResponseDTO{Code: 500, Message: "open upload failed"})
		return
	}
	defer src.Close()

	ct := fh.Header.Get("Content-Type")
	if ct == "" || ct == "application/octet-stream" {
		ct = service.ContentTypeForFilename(fh.Filename)
		if ct == "application/octet-stream" {
			// try extension
			name := fh.Filename
			switch {
			case len(name) > 4 && (name[len(name)-4:] == ".jpg" || name[len(name)-5:] == ".jpeg"):
				ct = "image/jpeg"
			case len(name) > 4 && name[len(name)-4:] == ".png":
				ct = "image/png"
			case len(name) > 5 && name[len(name)-5:] == ".webp":
				ct = "image/webp"
			case len(name) > 4 && name[len(name)-4:] == ".gif":
				ct = "image/gif"
			}
		}
	}

	publicPath, _, _, err := ctrl.media.SaveAvatar(uid, src, ct)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
		return
	}
	user, err := ctrl.authSvc.SetAvatar(uid, publicPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.APIResponseDTO{Code: 500, Message: err.Error()})
		return
	}
	url := fmt.Sprintf("%s?v=%d", publicPath, user.AvatarRev)
	c.JSON(http.StatusOK, dto.APIResponseDTO{
		Code:    200,
		Message: "avatar updated",
		Data: dto.AvatarUploadResponse{
			Avatar:    user.Avatar,
			AvatarRev: user.AvatarRev,
			URL:       url,
		},
	})
}

// GetAvatar serves a user's avatar image (public read for <img src>).
// GET /api/avatar/:user_id
func (ctrl *AuthController) GetAvatar(c *gin.Context) {
	if ctrl.media == nil {
		c.Status(http.StatusNotFound)
		return
	}
	uid := c.Param("user_id")
	path, ct, err := ctrl.media.ResolveAvatarPath(uid)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	// Short cache so avatar replace/delete propagates quickly (avoid stale solid placeholders).
	c.Header("Cache-Control", "public, max-age=60, must-revalidate")
	c.Header("Content-Type", ct)
	c.File(path)
}
