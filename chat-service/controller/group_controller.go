package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"ws-ex/dto"
	"ws-ex/service"
	"ws-ex/validate"
)

// GroupController handles create / list / dissolve of durable groups.
type GroupController struct {
	groups *service.GroupService
	media  *service.MediaService
}

func NewGroupController(groups *service.GroupService) *GroupController {
	return &GroupController{groups: groups}
}

// SetMedia enables group icon upload storage.
func (ctrl *GroupController) SetMedia(m *service.MediaService) {
	ctrl.media = m
}

func (ctrl *GroupController) me(c *gin.Context) uint {
	raw, _ := c.Get("user_id")
	return raw.(uint)
}

func writeValidateErr(c *gin.Context, err error) {
	code := http.StatusBadRequest
	c.JSON(code, dto.APIResponseDTO{Code: code, Message: err.Error()})
}

// Create POST /api/groups  { name, group_id? }
func (ctrl *GroupController) Create(c *gin.Context) {
	var body dto.CreateGroupRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		writeValidateErr(c, validate.JSONBody(err))
		return
	}
	// Service re-validates name / optional group_id.
	g, err := ctrl.groups.Create(ctrl.me(c), body.Name, body.GroupID)
	if err != nil {
		if validate.IsInvalid(err) {
			writeValidateErr(c, err)
			return
		}
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, dto.APIResponseDTO{Code: 201, Message: "group created", Data: g})
}

// ListMine GET /api/groups
func (ctrl *GroupController) ListMine(c *gin.Context) {
	list, err := ctrl.groups.ListMine(ctrl.me(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.APIResponseDTO{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.APIResponseDTO{
		Code: 200, Message: "success",
		Data: gin.H{"groups": list, "count": len(list)},
	})
}

// Search GET /api/groups/search?q=&limit=20
// Fuzzy match on group id / name for join autocomplete.
func (ctrl *GroupController) Search(c *gin.Context) {
	q, err := validate.SearchQuery(c.Query("q"))
	if err != nil {
		writeValidateErr(c, err)
		return
	}
	limit, err := validate.Limit(c.Query("limit"), 20, 50)
	if err != nil {
		writeValidateErr(c, err)
		return
	}
	list, err := ctrl.groups.Search(ctrl.me(c), q, limit)
	if err != nil {
		if validate.IsInvalid(err) {
			writeValidateErr(c, err)
			return
		}
		c.JSON(http.StatusInternalServerError, dto.APIResponseDTO{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.APIResponseDTO{
		Code: 200, Message: "success",
		Data: gin.H{"groups": list, "count": len(list), "q": q},
	})
}

// Get GET /api/groups/:group_id
func (ctrl *GroupController) Get(c *gin.Context) {
	gid, err := validate.GroupID(c.Param("group_id"), true)
	if err != nil {
		writeValidateErr(c, err)
		return
	}
	g, err := ctrl.groups.Get(ctrl.me(c), gid)
	if err != nil {
		c.JSON(http.StatusNotFound, dto.APIResponseDTO{Code: 404, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.APIResponseDTO{Code: 200, Message: "success", Data: g})
}

// Dissolve POST /api/groups/:group_id/dissolve — owner only (admins cannot dissolve).
func (ctrl *GroupController) Dissolve(c *gin.Context) {
	groupID, err := validate.GroupID(c.Param("group_id"), true)
	if err != nil {
		writeValidateErr(c, err)
		return
	}
	me := ctrl.me(c)
	memberIDs, name, err := ctrl.groups.Dissolve(me, groupID)
	if err != nil {
		if validate.IsInvalid(err) {
			writeValidateErr(c, err)
			return
		}
		code := http.StatusBadRequest
		if strings.Contains(err.Error(), "only the owner") {
			code = http.StatusForbidden
		}
		c.JSON(code, dto.APIResponseDTO{Code: code, Message: err.Error()})
		return
	}
	by := strconv.FormatUint(uint64(me), 10)
	ctrl.groups.NotifyDissolved(groupID, name, by, memberIDs)
	c.JSON(http.StatusOK, dto.APIResponseDTO{
		Code: 200, Message: "group dissolved",
		Data: gin.H{"group_id": groupID, "name": name},
	})
}

// Update PATCH /api/groups/:group_id  { name }  — owner or admin
func (ctrl *GroupController) Update(c *gin.Context) {
	groupID, err := validate.GroupID(c.Param("group_id"), true)
	if err != nil {
		writeValidateErr(c, err)
		return
	}
	var body dto.UpdateGroupRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		writeValidateErr(c, validate.JSONBody(err))
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "name is required"})
		return
	}
	g, err := ctrl.groups.UpdateName(ctrl.me(c), groupID, body.Name)
	if err != nil {
		if validate.IsInvalid(err) {
			writeValidateErr(c, err)
			return
		}
		code := http.StatusBadRequest
		if strings.Contains(err.Error(), "only owner") {
			code = http.StatusForbidden
		}
		c.JSON(code, dto.APIResponseDTO{Code: code, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.APIResponseDTO{Code: 200, Message: "group updated", Data: g})
}

// SetMemberRole PATCH /api/groups/:group_id/members/:user_id  { role: admin|member } — owner only
func (ctrl *GroupController) SetMemberRole(c *gin.Context) {
	groupID, err := validate.GroupID(c.Param("group_id"), true)
	if err != nil {
		writeValidateErr(c, err)
		return
	}
	targetStr, err := validate.UserIDStr(c.Param("user_id"), true)
	if err != nil {
		writeValidateErr(c, err)
		return
	}
	targetID, err := strconv.ParseUint(targetStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "invalid user_id"})
		return
	}
	var body dto.UpdateMemberRoleRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		writeValidateErr(c, validate.JSONBody(err))
		return
	}
	m, err := ctrl.groups.SetMemberRole(ctrl.me(c), groupID, uint(targetID), body.Role)
	if err != nil {
		code := http.StatusBadRequest
		if strings.Contains(err.Error(), "only the group owner") {
			code = http.StatusForbidden
		}
		c.JSON(code, dto.APIResponseDTO{Code: code, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.APIResponseDTO{Code: 200, Message: "role updated", Data: m})
}

// UploadAvatar POST /api/groups/:group_id/avatar  multipart field "file" (owner or admin).
func (ctrl *GroupController) UploadAvatar(c *gin.Context) {
	if ctrl.media == nil {
		c.JSON(http.StatusServiceUnavailable, dto.APIResponseDTO{Code: 503, Message: "media not configured"})
		return
	}
	groupID, err := validate.GroupID(c.Param("group_id"), true)
	if err != nil {
		writeValidateErr(c, err)
		return
	}
	me := ctrl.me(c)
	if !ctrl.groups.CanManageGroup(me, groupID) {
		c.JSON(http.StatusForbidden, dto.APIResponseDTO{Code: 403, Message: "only owner or admin can update the group icon"})
		return
	}

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
	}

	publicPath, _, _, err := ctrl.media.SaveGroupAvatar(groupID, src, ct)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
		return
	}
	g, err := ctrl.groups.SetAvatar(me, groupID, publicPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
		return
	}
	url := fmt.Sprintf("%s?v=%d", publicPath, g.AvatarRev)
	c.JSON(http.StatusOK, dto.APIResponseDTO{
		Code: 200, Message: "group icon updated",
		Data: gin.H{
			"group":      g,
			"avatar":     g.Avatar,
			"avatar_rev": g.AvatarRev,
			"url":        url,
		},
	})
}

// ListAnnouncements GET /api/groups/:group_id/announcements
func (ctrl *GroupController) ListAnnouncements(c *gin.Context) {
	groupID, err := validate.GroupID(c.Param("group_id"), true)
	if err != nil {
		writeValidateErr(c, err)
		return
	}
	list, err := ctrl.groups.ListAnnouncements(groupID)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.APIResponseDTO{
		Code: 200, Message: "success",
		Data: gin.H{"announcements": list, "count": len(list)},
	})
}

// AddAnnouncements POST /api/groups/:group_id/announcements
// Body: single {message_id, content, ...} or bulk {items:[{message_id,...}]} / {message_ids:[]}
func (ctrl *GroupController) AddAnnouncements(c *gin.Context) {
	groupID, err := validate.GroupID(c.Param("group_id"), true)
	if err != nil {
		writeValidateErr(c, err)
		return
	}
	var body dto.AddAnnouncementRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		writeValidateErr(c, validate.JSONBody(err))
		return
	}
	items := body.Items
	if len(items) == 0 && len(body.MessageIDs) > 0 {
		for _, id := range body.MessageIDs {
			items = append(items, dto.AddAnnouncementItem{MessageID: id})
		}
	}
	if len(items) == 0 && strings.TrimSpace(body.MessageID) != "" {
		items = append(items, dto.AddAnnouncementItem{
			MessageID:    body.MessageID,
			Content:      body.Content,
			ContentType:  body.ContentType,
			FromUserID:   body.FromUserID,
			FromUsername: body.FromUsername,
			MessageTS:    body.MessageTS,
		})
	}
	me := ctrl.me(c)
	list, err := ctrl.groups.AddAnnouncements(me, groupID, items)
	if err != nil {
		code := http.StatusBadRequest
		if strings.Contains(err.Error(), "only owner") {
			code = http.StatusForbidden
		}
		c.JSON(code, dto.APIResponseDTO{Code: code, Message: err.Error()})
		return
	}
	by := strconv.FormatUint(uint64(me), 10)
	action := "set"
	if len(list) > 1 {
		action = "set_bulk"
	}
	ctrl.groups.NotifyAnnouncement(groupID, by, action, list, "")
	c.JSON(http.StatusOK, dto.APIResponseDTO{
		Code: 200, Message: "announcement set",
		Data: gin.H{"announcements": list, "count": len(list)},
	})
}

// RemoveAnnouncement DELETE /api/groups/:group_id/announcements/:message_id
func (ctrl *GroupController) RemoveAnnouncement(c *gin.Context) {
	groupID, err := validate.GroupID(c.Param("group_id"), true)
	if err != nil {
		writeValidateErr(c, err)
		return
	}
	messageID := strings.TrimSpace(c.Param("message_id"))
	if messageID == "" {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: "message_id required"})
		return
	}
	me := ctrl.me(c)
	if err := ctrl.groups.RemoveAnnouncement(me, groupID, messageID); err != nil {
		code := http.StatusBadRequest
		if strings.Contains(err.Error(), "only owner") {
			code = http.StatusForbidden
		}
		c.JSON(code, dto.APIResponseDTO{Code: code, Message: err.Error()})
		return
	}
	by := strconv.FormatUint(uint64(me), 10)
	ctrl.groups.NotifyAnnouncement(groupID, by, "remove", nil, messageID)
	c.JSON(http.StatusOK, dto.APIResponseDTO{Code: 200, Message: "announcement removed"})
}

// GetAvatar GET /api/group-avatar/:group_id — public image for <img src>.
func (ctrl *GroupController) GetAvatar(c *gin.Context) {
	if ctrl.media == nil {
		c.Status(http.StatusNotFound)
		return
	}
	groupID, err := validate.GroupID(c.Param("group_id"), true)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	path, ct, err := ctrl.media.ResolveGroupAvatarPath(groupID)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "public, max-age=60, must-revalidate")
	c.Header("Content-Type", ct)
	c.File(path)
}
