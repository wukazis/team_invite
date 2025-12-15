package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
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
	"team-invite/internal/config"
	"team-invite/internal/database"
	"team-invite/internal/middleware"
	"team-invite/internal/models"
	"team-invite/internal/oauth"
	adminsvc "team-invite/internal/services/admin"
	invitesvc "team-invite/internal/services/invite"
	"team-invite/internal/services/teamstatus"
)

var allowedEnvKeys = []string{
	"SECRET_KEY",
	"ADMIN_PASSWORD",
	"JWT_SECRET",
	"POSTGRES_URL",
	"CF_TURNSTILE_SECRET_KEY",
	"CF_TURNSTILE_SITE_KEY",
	"LINUXDO_CLIENT_ID",
	"LINUXDO_CLIENT_SECRET",
	"LINUXDO_REDIRECT_URI",
}

type Handler struct {
	cfg        *config.Config
	store      *database.Store
	env        *adminsvc.EnvService
	jwt        *auth.Manager
	oauth      *oauth.Client
	inviter    *invitesvc.Service
	logger     *slog.Logger
	teamStatus *teamstatus.Service
	stateMu    sync.Mutex
	stateStore map[string]time.Time
}

func NewHandler(cfg *config.Config, store *database.Store, envSvc *adminsvc.EnvService, jwt *auth.Manager, oauthClient *oauth.Client, inviter *invitesvc.Service, teamStatus *teamstatus.Service, logger *slog.Logger) *Handler {
	return &Handler{
		cfg:        cfg,
		store:      store,
		env:        envSvc,
		jwt:        jwt,
		oauth:      oauthClient,
		inviter:    inviter,
		teamStatus: teamStatus,
		logger:     logger,
		stateStore: make(map[string]time.Time),
	}
}


func RegisterRoutes(r *gin.Engine, h *Handler, jwt *auth.Manager, adminIPs []string) {
	r.GET("/api/health", h.Health)
	r.GET("/api/turnstile/site-key", h.GetTurnstileSiteKey)

	r.GET("/api/oauth/login", h.OAuthLogin)
	r.GET("/api/oauth/callback", h.OAuthCallback)

	api := r.Group("/api")
	api.Use(middleware.JWT(jwt))

	api.GET("/state", h.UserState)
	api.POST("/invite", h.InviteSubmit)
	api.GET("/invite", h.InviteInfo)
	api.GET("/invite/resolve", h.ResolveInviteCode)
	api.GET("/team-accounts/status", h.PublicTeamAccountsStatus)

	admin := r.Group("/api/admin")
	admin.Use(middleware.AdminIPWhitelist(adminIPs))
	admin.POST("/login", h.AdminLogin)

	adminProtected := admin.Group("/")
	adminProtected.Use(middleware.JWT(jwt), middleware.RequireRole("admin"))
	adminProtected.GET("/env", h.EnvList)
	adminProtected.PUT("/env", h.EnvUpdate)
	adminProtected.GET("/users", h.AdminListUsers)
	adminProtected.PUT("/users/:id", h.AdminUpdateUser)
	adminProtected.GET("/invite-codes", h.AdminListInviteCodes)
	adminProtected.POST("/invite-codes", h.AdminCreateInviteCodes)
	adminProtected.PUT("/invite-codes/:id", h.AdminUpdateInviteCode)
	adminProtected.DELETE("/invite-codes/:id", h.AdminDeleteInviteCode)
	adminProtected.POST("/invite-codes/:id/assign", h.AdminAssignInviteCode)
	adminProtected.GET("/team-accounts", h.AdminListTeamAccounts)
	adminProtected.POST("/team-accounts", h.AdminCreateTeamAccount)
	adminProtected.PUT("/team-accounts/:id", h.AdminUpdateTeamAccount)
	adminProtected.DELETE("/team-accounts/:id", h.AdminDeleteTeamAccount)
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
		"invite": invitePayload,
	})
}


type inviteRequest struct {
	Email          string `json:"email" binding:"required,email"`
	Code           string `json:"code"`
	TeamAccountID  int64  `json:"teamAccountId"`
	TurnstileToken string `json:"turnstileToken"`
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
	var boundTeamID *int64
	if strings.TrimSpace(req.Code) != "" {
		codeRecord, err := h.store.GetInviteCodeByCode(c.Request.Context(), req.Code)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, database.ErrInviteNotFound) {
				status = http.StatusNotFound
			}
			c.JSON(status, gin.H{"error": "邀请码不可用"})
			return
		}
		if codeRecord.Used {
			c.JSON(http.StatusConflict, gin.H{"error": "邀请码已使用"})
			return
		}
		if codeRecord.TeamAccountID != nil {
			boundTeamID = codeRecord.TeamAccountID
		}
	}

	if h.cfg.TurnstileSecret != "" {
		if strings.TrimSpace(req.TurnstileToken) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请完成人机验证"})
			return
		}
		if err := h.verifyTurnstile(c.Request.Context(), req.TurnstileToken, clientIP(c)); err != nil {
			h.logger.Warn("turnstile verification failed", "user", claims.UserID, "error", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "人机验证失败"})
			return
		}
	}

	teamAccountID := req.TeamAccountID
	if boundTeamID != nil {
		teamAccountID = *boundTeamID
	}
	account, err := h.getTeamAccount(c.Request.Context(), teamAccountID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
			return h.inviter.SendWithAccount(ctx, *account, req.Email)
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
		return h.inviter.SendWithAccount(ctx, *account, req.Email)
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
	go h.teamStatus.RefreshAsync(c.Request.Context())
}

func (h *Handler) getTeamAccount(ctx context.Context, id int64) (*config.InviteAccount, error) {
	if id <= 0 {
		return nil, errors.New("请选择车位")
	}
	acc, err := h.store.GetTeamAccount(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("车位不存在")
	}
	if !acc.Enabled {
		return nil, fmt.Errorf("车位不可用")
	}
	if strings.TrimSpace(acc.AccountID) == "" || strings.TrimSpace(acc.AuthToken) == "" {
		return nil, fmt.Errorf("车位凭据未配置")
	}
	return &config.InviteAccount{
		AccountID:          acc.AccountID,
		AuthorizationToken: acc.AuthToken,
	}, nil
}

func (h *Handler) verifyTurnstile(ctx context.Context, token, ip string) error {
	if h.cfg.TurnstileSecret == "" {
		return nil
	}
	form := url.Values{}
	form.Set("secret", h.cfg.TurnstileSecret)
	form.Set("response", token)
	if net.ParseIP(ip) != nil {
		form.Set("remoteip", ip)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://challenges.cloudflare.com/turnstile/v0/siteverify", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var payload struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}
	if !payload.Success {
		return fmt.Errorf("turnstile failed")
	}
	return nil
}

func clientIP(c *gin.Context) string {
	ip := c.ClientIP()
	if ip == "" {
		return ""
	}
	return ip
}

func (h *Handler) refreshRuntimeConfig() {
	values, err := h.env.Read()
	if err != nil {
		h.logger.Warn("env reload failed", "error", err)
		return
	}
	if v, ok := values["CF_TURNSTILE_SITE_KEY"]; ok {
		h.cfg.TurnstileSiteKey = v
	}
	if v, ok := values["CF_TURNSTILE_SECRET_KEY"]; ok {
		h.cfg.TurnstileSecret = v
	}
	if v, ok := values["APP_BASE_URL"]; ok {
		h.cfg.AppBaseURL = v
	}
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

func (h *Handler) GetTurnstileSiteKey(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"siteKey": h.cfg.TurnstileSiteKey})
}

func (h *Handler) ResolveInviteCode(c *gin.Context) {
	code := strings.TrimSpace(c.Query("code"))
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少邀请码"})
		return
	}
	invite, err := h.store.GetInviteCodeByCode(c.Request.Context(), code)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, database.ErrInviteNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": "邀请码不可用"})
		return
	}
	if invite.Used {
		c.JSON(http.StatusConflict, gin.H{"error": "邀请码已使用"})
		return
	}
	response := gin.H{"teamAccountId": invite.TeamAccountID}
	if invite.TeamAccountID != nil {
		acc, accErr := h.store.GetTeamAccount(c.Request.Context(), *invite.TeamAccountID)
		if accErr == nil {
			response["teamAccountName"] = acc.Name
		}
	}
	c.JSON(http.StatusOK, response)
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
	h.refreshRuntimeConfig()
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
	if req.InviteStatus == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无更新内容"})
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
	if err := h.store.UpdateUser(c.Request.Context(), userID, statusPtr); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
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
	Count         int    `json:"count"`
	TeamAccountID *int64 `json:"teamAccountId"`
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
	var teamIDPtr *int64
	if req.TeamAccountID != nil && *req.TeamAccountID > 0 {
		acc, accErr := h.store.GetTeamAccount(c.Request.Context(), *req.TeamAccountID)
		if accErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "车账号不存在"})
			return
		}
		if !acc.Enabled {
			c.JSON(http.StatusBadRequest, gin.H{"error": "车账号已禁用"})
			return
		}
		teamIDPtr = req.TeamAccountID
	}

	codes, err := h.store.CreateInviteCodes(c.Request.Context(), req.Count, teamIDPtr)
	if err != nil {
		h.logger.Error("admin create invite codes failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"codes": codes})
}

type updateInviteCodeRequest struct {
	Used          bool    `json:"used"`
	UsedEmail     *string `json:"usedEmail"`
	TeamAccountID *int64  `json:"teamAccountId"`
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
		if req.TeamAccountID == nil || *req.TeamAccountID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请选择车位"})
			return
		}
		account, accErr := h.getTeamAccount(c.Request.Context(), *req.TeamAccountID)
		if accErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": accErr.Error()})
			return
		}
		email := strings.TrimSpace(*req.UsedEmail)
		if email == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请填写邮箱"})
			return
		}
		if err := h.inviter.SendWithAccount(c.Request.Context(), *account, email); err != nil {
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
