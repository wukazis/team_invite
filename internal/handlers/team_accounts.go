package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// PublicTeamAccountsStatus 公开接口，返回所有启用账号的状态（不含敏感信息）
func (h *Handler) PublicTeamAccountsStatus(c *gin.Context) {
	statuses := h.teamStatus.PublicStatuses()
	c.JSON(http.StatusOK, gin.H{"accounts": statuses})
}

// AdminListTeamAccounts 管理员接口，返回所有账号（含敏感信息）
func (h *Handler) AdminListTeamAccounts(c *gin.Context) {
	statuses := h.teamStatus.AdminStatuses()
	c.JSON(http.StatusOK, gin.H{"accounts": statuses})
}

type createTeamAccountRequest struct {
	Name      string `json:"name" binding:"required"`
	AccountID string `json:"accountId" binding:"required"`
	AuthToken string `json:"authToken" binding:"required"`
	MaxSeats  int    `json:"maxSeats"`
}

func (h *Handler) AdminCreateTeamAccount(c *gin.Context) {
	var req createTeamAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if req.MaxSeats <= 0 {
		req.MaxSeats = 50
	}

	acc, err := h.store.CreateTeamAccount(c.Request.Context(), req.Name, req.AccountID, req.AuthToken, req.MaxSeats)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"account": acc})
}

type updateTeamAccountRequest struct {
	Name      string `json:"name" binding:"required"`
	AccountID string `json:"accountId" binding:"required"`
	AuthToken string `json:"authToken" binding:"required"`
	MaxSeats  int    `json:"maxSeats"`
	Enabled   bool   `json:"enabled"`
}

func (h *Handler) AdminUpdateTeamAccount(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID无效"})
		return
	}

	var req updateTeamAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if req.MaxSeats <= 0 {
		req.MaxSeats = 50
	}

	if err := h.store.UpdateTeamAccount(c.Request.Context(), id, req.Name, req.AccountID, req.AuthToken, req.MaxSeats, req.Enabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) AdminDeleteTeamAccount(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID无效"})
		return
	}

	if err := h.store.DeleteTeamAccount(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}
