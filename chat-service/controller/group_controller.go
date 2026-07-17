package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"ws-ex/dto"
	"ws-ex/service"
)

// GroupController handles create / list / dissolve of durable groups.
type GroupController struct {
	groups *service.GroupService
}

func NewGroupController(groups *service.GroupService) *GroupController {
	return &GroupController{groups: groups}
}

func (ctrl *GroupController) me(c *gin.Context) uint {
	raw, _ := c.Get("user_id")
	return raw.(uint)
}

// Create POST /api/groups  { name?, group_id? }
func (ctrl *GroupController) Create(c *gin.Context) {
	var body dto.CreateGroupRequest
	_ = c.ShouldBindJSON(&body)
	g, err := ctrl.groups.Create(ctrl.me(c), body.Name, body.GroupID)
	if err != nil {
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

// Get GET /api/groups/:group_id
func (ctrl *GroupController) Get(c *gin.Context) {
	g, err := ctrl.groups.Get(ctrl.me(c), c.Param("group_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, dto.APIResponseDTO{Code: 404, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.APIResponseDTO{Code: 200, Message: "success", Data: g})
}

// Dissolve POST /api/groups/:group_id/dissolve
func (ctrl *GroupController) Dissolve(c *gin.Context) {
	groupID := c.Param("group_id")
	me := ctrl.me(c)
	memberIDs, name, err := ctrl.groups.Dissolve(me, groupID)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{Code: 400, Message: err.Error()})
		return
	}
	by := strconv.FormatUint(uint64(me), 10)
	ctrl.groups.NotifyDissolved(groupID, name, by, memberIDs)
	c.JSON(http.StatusOK, dto.APIResponseDTO{
		Code: 200, Message: "group dissolved",
		Data: gin.H{"group_id": groupID, "name": name},
	})
}
