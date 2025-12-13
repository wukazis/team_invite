package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"

	"team-invite/internal/auth"
	"team-invite/internal/cache"
	"team-invite/internal/config"
	"team-invite/internal/database"
	"team-invite/internal/middleware"
	"team-invite/internal/models"
	"team-invite/internal/oauth"
	adminsvc "team-invite/internal/services/admin"
	"team-invite/internal/services/draw"
	invitesvc "team-invite/internal/services/invite"
)

var allowedEnvKeys = []string{
	"SECRET_KEY",
	"ADMIN_PASSWORD",
	"AUTHORIZATION_TOKEN",
	"INVITE_ACCOUNTS",
	"INVITE_STRATEGY",
	"INVITE_ACTIVE_ACCOUNT_ID",
	"JWT_SECRET",
	"POSTGRES_URL",
	"ACCOUNT_ID",
	"CF_TURNSTILE_SECRET_KEY",
	"CF_TURNSTILE_SITE_KEY",
	"LINUXDO_CLIENT_ID",
	"LINUXDO_CLIENT_SECRET",
	"LINUXDO_REDIRECT_URI",
}

type Handler struct {
	cfg        *config.Config
	store      *database.Store
	draw       *draw.Service
	env        *adminsvc.EnvService
	jwt        *auth.Manager
	oauth      *oauth.Client
	cache      *cache.PrizeConfigCache
	inviter    *invitesvc.Service
	logger     *slog.Logger
	stateMu    sync.Mutex
	stateStore map[string]time.Time
}

func NewHandler(cfg *config.Config, store *database.Store, drawSvc *draw.Service, envSvc *adminsvc.EnvService, jwt *auth.Manager, oauthClient *oauth.Client, prizeCache *cache.PrizeConfigCache, inviter *invitesvc.Service, logger *slog.Logger) *Handler {
	return &Handler{
		cfg:        cfg,
		store:      store,
		draw:       drawSvc,
		env:        envSvc,
		jwt:        jwt,
		oauth:      oauthClient,
		cache:      prizeCache,
		inviter:    inviter,
		logger:     logger,
		stateStore: make(map[string]time.Time),
	}
}

func RegisterRoutes(r *gin.Engine, h *Handler, jwt *auth.Manager, adminIPs []string) {
	r.GET("/api/health", h.Health)
	r.GET("/api/quota/public", h.PublicQuota)

	r.GET("/api/oauth/login", h.OAuthLogin)
	r.GET("/api/oauth/callback", h.OAuthCallback)

	api := r.Group("/api")
	api.Use(middleware.JWT(jwt))

	api.GET("/state", h.UserState)
	api.GET("/quota", h.Quota)
	api.POST("/spins", h.Spin)
	api.POST("/invite", h.InviteSubmit)
	api.GET("/invite", h.InviteInfo)

	admin := r.Group("/api/admin")
	admin.Use(middleware.AdminIPWhitelist(adminIPs))
	admin.POST("/login", h.AdminLogin)

	adminProtected := admin.Group("/")
	adminProtected.Use(middleware.JWT(jwt), middleware.RequireRole("admin"))
	adminProtected.GET("/env", h.EnvList)
	adminProtected.PUT("/env", h.EnvUpdate)
	adminProtected.POST("/quota", h.UpdateQuota)
	adminProtected.POST("/quota/schedule", h.ScheduleQuota)
	adminProtected.POST("/users/reset", h.AdminResetUsers)
	adminProtected.GET("/users", h.AdminListUsers)
	adminProtected.PUT("/users/:id", h.AdminUpdateUser)
	adminProtected.GET("/spins", h.AdminListSpins)
	adminProtected.GET("/invite-codes", h.AdminListInviteCodes)
	adminProtected.POST("/invite-codes", h.AdminCreateInviteCodes)
	adminProtected.PUT("/invite-codes/:id", h.AdminUpdateInviteCode)
	adminProtected.DELETE("/invite-codes/:id", h.AdminDeleteInviteCode)
	adminProtected.POST("/invite-codes/:id/assign", h.AdminAssignInviteCode)
	adminProtected.GET("/stats/hourly", h.HourlyStats)
	adminProtected.GET("/prize-config", h.GetPrizeConfig)
	adminProtected.PUT("/prize-config", h.UpdatePrizeConfig)
}

func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "timestamp": time.Now().UTC()})
}

func (h *Handler) OAuthLogin(c *gin.Context) {
	state := uuid.NewString()
	h.stateMu.Lock()
	h.stateStore[state] = time.Now().Add(10 * time.Minute)
	h.stateMu.Unlock()
	c.JSON(http.StatusOK, gin.H{
		"authUrl": h.oauth.AuthURL(state),
		"state":   state,
	})
}

func (h *Handler) OAuthCallback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")
	if code == "" || state == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing code/state"})
		return
	}
	if !h.consumeState(state) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid state"})
		return
	}
	token, err := h.oauth.Exchange(c.Request.Context(), code)
	if err != nil {
		h.logger.Error("token exchange failed", "error", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "oauth exchange failed"})
		return
	}
	profile, err := h.oauth.FetchProfile(c.Request.Context(), token.AccessToken)
	if err != nil {
		h.logger.Error("profile fetch failed", "error", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "profile fetch failed"})
		return
	}
	user, err := h.store.UpsertUserFromOAuth(c.Request.Context(), profile)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "user sync failed"})
		return
	}
	jwtToken, err := h.jwt.IssueToken(user.ID, user.Username, "user", 24*time.Hour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token issue failed"})
		return
	}
	target := fmt.Sprintf("%s/auth/callback?token=%s", h.cfg.AppBaseURL, url.QueryEscape(jwtToken))
	c.Redirect(http.StatusFound, target)
}

func (h *Handler) consumeState(state string) bool {
	h.stateMu.Lock()
	defer h.stateMu.Unlock()
	expiry, ok := h.stateStore[state]
	if !ok {
		return false
	}
	delete(h.stateStore, state)
	return time.Now().Before(expiry)
}

func (h *Handler) UserState(c *gin.Context) {
	claims := middleware.ClaimsFromContext(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing claims"})
		return
	}
	user, err := h.store.GetUserByID(c.Request.Context(), claims.UserID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	quota, err := h.store.GetQuota(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "quota read failed"})
		return
	}
	invite, _ := h.store.LatestInviteForUser(c.Request.Context(), user.ID)
	var invitePayload any
	if invite != nil {
		invitePayload = gin.H{
			"code":      invite.Code,
			"used":      invite.Used,
			"usedEmail": invite.UsedEmail,
			"usedAt":    invite.UsedAt,
			"createdAt": invite.CreatedAt,
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"user":   user,
		"quota":  quota,
		"invite": invitePayload,
	})
}

func inviteIf[T any](invite *models.InviteCode, fn func(*models.InviteCode) T) T {
	var zero T
	if invite == nil {
		return zero
	}
	return fn(invite)
}

func (h *Handler) Quota(c *gin.Context) {
	q, err := h.store.GetQuota(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "quota read failed"})
		return
	}
	now := time.Now().UTC()
	schedule, err := h.store.LoadQuotaSchedule(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "quota schedule read failed"})
		return
	}
	var schedulePayload any
	if schedule != nil && now.Before(schedule.ApplyAt) {
		schedulePayload = gin.H{
			"applyAt":   schedule.ApplyAt.UTC(),
			"target":    schedule.Target,
			"message":   schedule.Message,
			"author":    schedule.Author,
			"createdAt": schedule.CreatedAt.UTC(),
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"quota":      q,
		"updatedAt":  now,
		"serverTime": now,
		"schedule":   schedulePayload,
	})
}

func (h *Handler) PublicQuota(c *gin.Context) {
	h.Quota(c)
}

type spinRequest struct {
	SpinID string `json:"spinId"`
}

func (h *Handler) Spin(c *gin.Context) {
	var req spinRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	claims := middleware.ClaimsFromContext(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing claims"})
		return
	}
	user, err := h.store.GetUserByID(c.Request.Context(), claims.UserID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if user.InviteStatus != models.UserStatusNone {
		message := "您已有待填写的邀请码"
		if user.InviteStatus == models.UserStatusCompleted {
			message = "您已完成邀请码领取"
		}
		c.JSON(http.StatusConflict, gin.H{"error": message})
		return
	}
	outcome, err := h.draw.Spin(c.Request.Context(), user, req.SpinID)
	if err != nil {
		switch {
		case errors.Is(err, database.ErrNoChances):
			c.JSON(http.StatusConflict, gin.H{"error": "no chances"})
		case errors.Is(err, database.ErrQuotaEmpty):
			c.JSON(http.StatusConflict, gin.H{"error": "no quota"})
		default:
			h.logger.Error("spin failed", "error", err, "user", user.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "spin failed"})
		}
		return
	}
	c.JSON(http.StatusOK, outcome)
}

type inviteRequest struct {
	Email string `json:"email" binding:"required,email"`
	Code  string `json:"code"`
}

func (h *Handler) InviteSubmit(c *gin.Context) {
	var req inviteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请填写有效邮箱"})
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	claims := middleware.ClaimsFromContext(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing claims"})
		return
	}

	invite, err := h.store.LatestInviteForUser(c.Request.Context(), claims.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}

	if invite == nil {
		if strings.TrimSpace(req.Code) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "没有可用的邀请码"})
			return
		}
		sendFunc := func(ctx context.Context) error {
			return h.inviter.Send(ctx, req.Email)
		}
		if err := h.store.CompleteInviteSubmissionByCode(c.Request.Context(), req.Code, claims.UserID, req.Email, sendFunc); err != nil {
			var sendErr *invitesvc.SendError
			switch {
			case errors.Is(err, database.ErrInviteNotFound):
				c.JSON(http.StatusBadRequest, gin.H{"error": "邀请码不存在"})
			case errors.Is(err, database.ErrAlreadyClaimed):
				c.JSON(http.StatusConflict, gin.H{"error": "邀请码已使用"})
			case errors.Is(err, database.ErrInviteAssigned):
				c.JSON(http.StatusConflict, gin.H{"error": "邀请码不可用"})
			case errors.As(err, &sendErr):
				h.logger.Error("invite send failed", "status", sendErr.StatusCode, "body", sendErr.Body, "user", claims.UserID)
				if sendErr.StatusCode == 0 {
					c.JSON(http.StatusServiceUnavailable, gin.H{"error": "邀请发送未配置"})
					return
				}
				c.JSON(http.StatusBadGateway, gin.H{"error": "邀请发送失败，请稍后再试"})
			default:
				h.logger.Error("invite submit failed", "error", err, "user", claims.UserID)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "提交失败"})
			}
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "submitted"})
		return
	}

	if invite.Used {
		c.JSON(http.StatusConflict, gin.H{"error": "已经完成"})
		return
	}
	sendFunc := func(ctx context.Context) error {
		return h.inviter.Send(ctx, req.Email)
	}
	if err := h.store.CompleteInviteSubmission(c.Request.Context(), invite.ID, req.Email, sendFunc); err != nil {
		var sendErr *invitesvc.SendError
		switch {
		case errors.Is(err, database.ErrAlreadyClaimed):
			c.JSON(http.StatusConflict, gin.H{"error": "已经完成"})
		case errors.As(err, &sendErr):
			h.logger.Error("invite send failed", "status", sendErr.StatusCode, "body", sendErr.Body, "user", claims.UserID)
			if sendErr.StatusCode == 0 {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": "邀请发送未配置"})
				return
			}
			c.JSON(http.StatusBadGateway, gin.H{"error": "邀请发送失败，请稍后再试"})
		default:
			h.logger.Error("invite submit failed", "error", err, "user", claims.UserID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "提交失败"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "submitted"})
}

func (h *Handler) InviteInfo(c *gin.Context) {
	claims := middleware.ClaimsFromContext(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing claims"})
		return
	}
	invite, err := h.store.LatestInviteForUser(c.Request.Context(), claims.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"invite": invite})
}

type adminLoginRequest struct {
	Password string `json:"password"`
	Code     string `json:"code"`
}

func (h *Handler) AdminLogin(c *gin.Context) {
	var req adminLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if req.Password != h.cfg.AdminPassword {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	if ok := totp.Validate(req.Code, h.cfg.AdminTOTPSecret); !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid totp"})
		return
	}
	token, err := h.jwt.IssueToken(0, "admin", "admin", 4*time.Hour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token})
}

func (h *Handler) EnvList(c *gin.Context) {
	values, err := h.env.Read()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "env read failed"})
		return
	}
	result := map[string]string{}
	for _, key := range allowedEnvKeys {
		if v, ok := values[key]; ok {
			result[key] = v
		}
	}
	c.JSON(http.StatusOK, gin.H{"env": result})
}

func (h *Handler) EnvUpdate(c *gin.Context) {
	payload := map[string]string{}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	updates := map[string]string{}
	for key, value := range payload {
		if !contains(allowedEnvKeys, key) {
			continue
		}
		updates[key] = value
	}
	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no valid keys"})
		return
	}
	if err := h.env.Update(updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"updated": len(updates)})
}

func contains(slice []string, key string) bool {
	for _, item := range slice {
		if item == key {
			return true
		}
	}
	return false
}

type quotaUpdateRequest struct {
	Value int `json:"value"`
}

func (h *Handler) UpdateQuota(c *gin.Context) {
	var req quotaUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Value < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "quota 必须是非负整数"})
		return
	}
	if err := h.store.UpdateQuota(c.Request.Context(), req.Value); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "quota 更新失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"quota": req.Value})
}

type quotaScheduleRequest struct {
	Target       int       `json:"target"`
	ApplyAt      time.Time `json:"applyAt"`
	DelayMinutes int       `json:"delayMinutes"`
	Message      string    `json:"message"`
}

func (h *Handler) ScheduleQuota(c *gin.Context) {
	var req quotaScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Target < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	var applyAt time.Time
	if !req.ApplyAt.IsZero() {
		applyAt = req.ApplyAt
	} else if req.DelayMinutes > 0 {
		applyAt = time.Now().Add(time.Duration(req.DelayMinutes) * time.Minute)
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "需要指定时间"})
		return
	}
	claims := middleware.ClaimsFromContext(c)
	author := "admin"
	if claims != nil && claims.Username != "" {
		author = claims.Username
	}
	schedule := database.QuotaSchedule{
		Target:    req.Target,
		ApplyAt:   applyAt,
		Author:    author,
		Message:   req.Message,
		CreatedAt: time.Now(),
	}
	if err := h.store.SaveQuotaSchedule(c.Request.Context(), schedule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "计划保存失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"scheduled": schedule})
}

func (h *Handler) AdminResetUsers(c *gin.Context) {
	reset, err := h.store.ResetChancesForUnwon(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "重置失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"resetChances": reset,
	})
}

func (h *Handler) AdminListUsers(c *gin.Context) {
	limit, offset := parsePagination(c, 50, 0)
	users, total, err := h.store.ListUsers(c.Request.Context(), limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法读取用户列表"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"users": users,
		"total": total,
	})
}

type updateUserRequest struct {
	Chances      *int `json:"chances"`
	InviteStatus *int `json:"inviteStatus"`
}

func (h *Handler) AdminUpdateUser(c *gin.Context) {
	idRaw := c.Param("id")
	userID, err := strconv.ParseInt(idRaw, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户ID无效"})
		return
	}
	var req updateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if req.Chances == nil && req.InviteStatus == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无更新内容"})
		return
	}
	if req.Chances != nil && *req.Chances < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "次数需为非负"})
		return
	}
	var statusPtr *models.UserStatus
	if req.InviteStatus != nil {
		status := models.UserStatus(*req.InviteStatus)
		if status < models.UserStatusNone || status > models.UserStatusCompleted {
			c.JSON(http.StatusBadRequest, gin.H{"error": "状态不合法"})
			return
		}
		statusPtr = &status
	}
	if err := h.store.UpdateUser(c.Request.Context(), userID, req.Chances, statusPtr); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) AdminListSpins(c *gin.Context) {
	limit, offset := parsePagination(c, 50, 0)
	records, total, err := h.store.ListSpinRecords(c.Request.Context(), limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法读取抽奖记录"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"records": records,
		"total":   total,
	})
}

func (h *Handler) AdminListInviteCodes(c *gin.Context) {
	limit, offset := parsePagination(c, 50, 0)
	codes, total, err := h.store.ListInviteCodes(c.Request.Context(), limit, offset)
	if err != nil {
		h.logger.Error("admin list invite codes failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法读取邀请码"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"codes": codes,
		"total": total,
	})
}

type createInviteCodesRequest struct {
	Count int `json:"count"`
}

func (h *Handler) AdminCreateInviteCodes(c *gin.Context) {
	var req createInviteCodesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if req.Count <= 0 {
		req.Count = 1
	}
	if req.Count > 10 {
		req.Count = 10
	}
	codes, err := h.store.CreateInviteCodes(c.Request.Context(), req.Count)
	if err != nil {
		h.logger.Error("admin create invite codes failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"codes": codes})
}

type updateInviteCodeRequest struct {
	Used      bool    `json:"used"`
	UsedEmail *string `json:"usedEmail"`
}

func (h *Handler) AdminUpdateInviteCode(c *gin.Context) {
	idRaw := c.Param("id")
	codeID, err := strconv.ParseInt(idRaw, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "邀请码ID无效"})
		return
	}
	var req updateInviteCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if req.Used {
		if req.UsedEmail == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请填写邮箱"})
			return
		}
		email := strings.TrimSpace(*req.UsedEmail)
		if email == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请填写邮箱"})
			return
		}
		if err := h.inviter.Send(c.Request.Context(), email); err != nil {
			var sendErr *invitesvc.SendError
			if errors.As(err, &sendErr) {
				h.logger.Error("admin invite send failed", "status", sendErr.StatusCode, "body", sendErr.Body, "codeID", codeID)
				c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("发送失败(%d)", sendErr.StatusCode)})
				return
			}
			h.logger.Error("admin invite send failed", "error", err, "codeID", codeID)
			c.JSON(http.StatusBadGateway, gin.H{"error": "发送失败，请稍后再试"})
			return
		}
		req.UsedEmail = &email
	}
	if err := h.store.UpdateInviteCode(c.Request.Context(), codeID, req.Used, req.UsedEmail); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败"})
		return
	}
	status := "ok"
	if req.Used {
		status = "sent"
	}
	c.JSON(http.StatusOK, gin.H{"status": status})
}

func (h *Handler) AdminDeleteInviteCode(c *gin.Context) {
	idRaw := c.Param("id")
	codeID, err := strconv.ParseInt(idRaw, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "邀请码ID无效"})
		return
	}
	if err := h.store.DeleteInviteCode(c.Request.Context(), codeID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

type assignInviteCodeRequest struct {
	UserID int64 `json:"userId"`
}

func (h *Handler) AdminAssignInviteCode(c *gin.Context) {
	idRaw := c.Param("id")
	codeID, err := strconv.ParseInt(idRaw, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "邀请码ID无效"})
		return
	}
	var req assignInviteCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.UserID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入有效的用户ID"})
		return
	}
	if err := h.store.AssignInviteCode(c.Request.Context(), codeID, req.UserID); err != nil {
		switch {
		case errors.Is(err, database.ErrAlreadyClaimed):
			c.JSON(http.StatusConflict, gin.H{"error": "邀请码已使用"})
		case errors.Is(err, database.ErrInviteAssigned):
			c.JSON(http.StatusConflict, gin.H{"error": "邀请码已绑定用户"})
		default:
			h.logger.Error("assign invite failed", "error", err, "codeID", codeID, "userID", req.UserID)
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "assigned"})
}

func parsePagination(c *gin.Context, defaultLimit, defaultOffset int) (int, int) {
	limit := defaultLimit
	offset := defaultOffset
	if v := c.Query("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}
	if v := c.Query("offset"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 0 {
			offset = parsed
		}
	}
	return limit, offset
}

func (h *Handler) HourlyStats(c *gin.Context) {
	since := time.Now().Add(-24 * time.Hour)
	stats, overview, err := h.store.HourlyStats(c.Request.Context(), since)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "统计失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"overview": overview,
		"stats":    stats,
	})
}

func (h *Handler) UpdatePrizeConfig(c *gin.Context) {
	var payload []models.PrizeConfigItem
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "配置格式错误"})
		return
	}
	if err := validatePrizeConfig(payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.store.UpdatePrizeConfig(c.Request.Context(), payload); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败"})
		return
	}
	h.cache.Invalidate()
	c.JSON(http.StatusOK, gin.H{"updated": len(payload)})
}

func (h *Handler) GetPrizeConfig(c *gin.Context) {
	items, err := h.store.LoadPrizeConfig(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func validatePrizeConfig(items []models.PrizeConfigItem) error {
	if len(items) == 0 {
		return errors.New("至少需要一个奖项")
	}
	var total float64
	for _, item := range items {
		if !allowedPrizeType(item.Type) {
			return fmt.Errorf("不支持的类型: %s", item.Type)
		}
		if item.Probability <= 0 {
			return fmt.Errorf("概率必须大于 0 (%s)", item.Name)
		}
		total += item.Probability
	}
	if math.Abs(total-1) > 0.01 {
		return fmt.Errorf("概率总和应接近 1，当前 %.2f", total)
	}
	return nil
}

func allowedPrizeType(t string) bool {
	switch t {
	case "win", "retry", "lose":
		return true
	default:
		return false
	}
}
