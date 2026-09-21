package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/cache"
	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
)

var (
	ErrAdminNotFound      = errors.New("admin not found")
	ErrUsernameTaken      = errors.New("username already exists")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrLastAdmin          = errors.New("cannot delete the last admin")
	ErrInvalidUsername    = errors.New("invalid username")
	ErrWeakPassword       = errors.New("password must be at least 8 characters")
	ErrInvalidToken       = errors.New("invalid token")
	ErrCacheUnavailable   = errors.New("session cache is unavailable")
	ErrTOTPRateLimited    = errors.New("totp rate limited")
	ErrWebAuthnNotReady   = errors.New("webauthn is not configured")
	ErrInvalidStage       = errors.New("invalid login stage")
	ErrTOTPNotConfirmed   = errors.New("totp is not confirmed")
	ErrPasskeyNotFound    = errors.New("passkey not found")
	ErrRecoveryNotAcked   = errors.New("recovery codes must be acknowledged")
)

var usernameRE = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,64}$`)

const (
	minPasswordLen    = 8
	lastLoginIPMaxLen = 64
	defaultIdle       = 72 * time.Hour
	defaultPendingTTL = 10 * time.Minute
	defaultTOTPMax    = 10
	totpPeriodSeconds = 30
	recoveryCodeCount = 10
	recoveryCodeChars = 8
	crockfordAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	totpIssuer        = "KiriVers"

	LoginStatusPending  = "pending"
	LoginStatusComplete = "complete"

	StageEnrollTOTP      = "enroll_totp"
	StageAckRecovery     = "ack_recovery"
	StageOptionalPasskey = "optional_passkey"
	StageSecondFactor    = "second_factor"
)

// AdminServiceOptions 注入管理员仓储、2FA 仓储、会话缓存与安全配置。
type AdminServiceOptions struct {
	Store           repository.AdminStore
	TwoFA           repository.Admin2FAStore
	Cache           cache.Store
	SessionIdle     time.Duration
	PendingTTL      time.Duration
	TOTPMaxAttempts int
	WebAuthnRPID    string
	WebAuthnOrigins []string
}

// AdminService 处理后台管理员账号、2FA 与缓存会话。
type AdminService struct {
	store       repository.AdminStore
	twoFA       repository.Admin2FAStore
	cache       cache.Store
	idle        time.Duration
	pendingTTL  time.Duration
	totpMax     int
	webauthnRP  string
	webauthnOrg []string
}

// NewAdminService 构造服务。idle/pending/TOTP 阈值 <= 0 时使用产品默认值。
func NewAdminService(opts AdminServiceOptions) *AdminService {
	idle := opts.SessionIdle
	if idle <= 0 {
		idle = defaultIdle
	}
	pending := opts.PendingTTL
	if pending <= 0 {
		pending = defaultPendingTTL
	}
	maxAttempts := opts.TOTPMaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultTOTPMax
	}
	origins := append([]string(nil), opts.WebAuthnOrigins...)
	return &AdminService{
		store:       opts.Store,
		twoFA:       opts.TwoFA,
		cache:       opts.Cache,
		idle:        idle,
		pendingTTL:  pending,
		totpMax:     maxAttempts,
		webauthnRP:  strings.TrimSpace(opts.WebAuthnRPID),
		webauthnOrg: origins,
	}
}

// NewTestAdminService 用内存缓存构造服务，供单测获取完整会话。
func NewTestAdminService(store repository.AdminStore) *AdminService {
	mem, err := cache.Open(cache.Options{Driver: "memory"})
	if err != nil {
		panic(err)
	}
	twoFA, ok := store.(repository.Admin2FAStore)
	if !ok {
		panic("admin store must implement Admin2FAStore")
	}
	return NewAdminService(AdminServiceOptions{
		Store: store,
		TwoFA: twoFA,
		Cache: mem,
	})
}

// Create 创建平台管理员。password 为明文，入库只存 bcrypt。
func (s *AdminService) Create(ctx context.Context, username, password string) (*model.Admin, error) {
	return s.CreateAccount(ctx, username, password, true)
}

// CreateAccount 创建账号。platform=false 用于项目成员流程。
func (s *AdminService) CreateAccount(ctx context.Context, username, password string, platform bool) (*model.Admin, error) {
	username = strings.TrimSpace(username)
	if err := validateUsername(username); err != nil {
		return nil, err
	}
	if err := validatePassword(password); err != nil {
		return nil, err
	}
	if _, err := s.store.GetByUsername(ctx, username); err == nil {
		return nil, ErrUsernameTaken
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	admin := &model.Admin{
		Username:        username,
		PasswordHash:    string(hash),
		IsPlatformAdmin: platform,
	}
	if err := s.store.Create(ctx, admin); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrUsernameTaken
		}
		return nil, err
	}
	return admin, nil
}

// GetByUsername 按登录名读取。
func (s *AdminService) GetByUsername(ctx context.Context, username string) (*model.Admin, error) {
	admin, err := s.store.GetByUsername(ctx, strings.TrimSpace(username))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAdminNotFound
	}
	return admin, err
}

// List 返回全部管理员（不含密码哈希）。
func (s *AdminService) List(ctx context.Context) ([]model.Admin, error) {
	return s.store.List(ctx)
}

// Get 按 ID 读取。
func (s *AdminService) Get(ctx context.Context, id uuid.UUID) (*model.Admin, error) {
	admin, err := s.store.GetByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAdminNotFound
	}
	return admin, err
}

// Update 修改用户名和/或密码。空字符串表示不改该项。
func (s *AdminService) Update(ctx context.Context, id uuid.UUID, username, password string) (*model.Admin, error) {
	admin, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if username != "" {
		username = strings.TrimSpace(username)
		if err := validateUsername(username); err != nil {
			return nil, err
		}
		if username != admin.Username {
			if _, err := s.store.GetByUsername(ctx, username); err == nil {
				return nil, ErrUsernameTaken
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			}
			admin.Username = username
		}
	}
	if password != "" {
		if err := validatePassword(password); err != nil {
			return nil, err
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("hash password: %w", err)
		}
		admin.PasswordHash = string(hash)
	}
	if err := s.store.Save(ctx, admin); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrUsernameTaken
		}
		return nil, err
	}
	return admin, nil
}

// Delete 删除管理员。禁止删掉最后一个账号。同时清除库中 2FA 凭证，不碰缓存会话。
func (s *AdminService) Delete(ctx context.Context, id uuid.UUID) error {
	if _, err := s.Get(ctx, id); err != nil {
		return err
	}
	n, err := s.store.Count(ctx)
	if err != nil {
		return err
	}
	if n <= 1 {
		return ErrLastAdmin
	}
	if err := s.clear2FARows(ctx, id); err != nil {
		return err
	}
	if err := s.store.Delete(ctx, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAdminNotFound
		}
		return err
	}
	return nil
}

// DeleteByUsername 供 CLI 使用。
func (s *AdminService) DeleteByUsername(ctx context.Context, username string) error {
	admin, err := s.store.GetByUsername(ctx, strings.TrimSpace(username))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrAdminNotFound
	}
	if err != nil {
		return err
	}
	return s.Delete(ctx, admin.ID)
}

// ResetPassword 按用户名重置密码。
func (s *AdminService) ResetPassword(ctx context.Context, username, password string) error {
	admin, err := s.store.GetByUsername(ctx, strings.TrimSpace(username))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrAdminNotFound
	}
	if err != nil {
		return err
	}
	_, err = s.Update(ctx, admin.ID, "", password)
	return err
}

// Clear2FA 按用户名删除库中 TOTP、Passkey 与恢复码，不删除缓存会话。
func (s *AdminService) Clear2FA(ctx context.Context, username string) error {
	admin, err := s.store.GetByUsername(ctx, strings.TrimSpace(username))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrAdminNotFound
	}
	if err != nil {
		return err
	}
	return s.clear2FARows(ctx, admin.ID)
}

func (s *AdminService) clear2FARows(ctx context.Context, adminID uuid.UUID) error {
	if s.twoFA == nil {
		return nil
	}
	if err := s.twoFA.DeleteTOTP(ctx, adminID); err != nil {
		return err
	}
	if err := s.twoFA.DeletePasskeysByAdmin(ctx, adminID); err != nil {
		return err
	}
	return s.twoFA.DeleteRecoveryCodes(ctx, adminID)
}

// LoginResult 是密码登录或 2FA 步骤的响应。
type LoginResult struct {
	Status        string
	Token         string
	PendingToken  string
	Stage         string
	ExpiresIn     int64
	Admin         *model.Admin
	RecoveryCodes []string
	Secret        string
	OTPAuthURL    string
	Passkeys      []PasskeyPublic
	TOTPEnabled   bool
	RecoveryLeft  int
	WebAuthn      json.RawMessage
	// HasPasskey 表示该账号已注册至少一把 Passkey。仅 Login 在 second_factor 时填充；不写入 pending 缓存。
	HasPasskey bool
}

// Login 校验密码并签发受限 token。不写 last-login，不签发完整会话。
func (s *AdminService) Login(ctx context.Context, username, password, clientIP string) (*LoginResult, error) {
	admin, err := s.store.GetByUsername(ctx, strings.TrimSpace(username))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(password)) != nil {
		return nil, ErrInvalidCredentials
	}
	stage := StageEnrollTOTP
	if row, err := s.twoFA.GetTOTP(ctx, admin.ID); err == nil && row.Confirmed() {
		stage = StageSecondFactor
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	out, err := s.issuePending(ctx, admin, stage, clientIP)
	if err != nil {
		return nil, err
	}
	if stage == StageSecondFactor {
		list, err := s.ListPasskeys(ctx, admin.ID)
		if err != nil {
			return nil, err
		}
		out.HasPasskey = len(list) > 0
	}
	return out, nil
}

// CompleteLogin 走完强制绑定或第二因素，供单测获取完整会话。
func (s *AdminService) CompleteLogin(ctx context.Context, username, password, clientIP string) (*LoginResult, error) {
	out, err := s.Login(ctx, username, password, clientIP)
	if err != nil {
		return nil, err
	}
	if out.Status == LoginStatusComplete {
		return out, nil
	}
	token := out.PendingToken
	switch out.Stage {
	case StageEnrollTOTP:
		setup, err := s.SetupTOTP(ctx, token)
		if err != nil {
			return nil, err
		}
		code, err := totp.GenerateCode(setup.Secret, time.Now())
		if err != nil {
			return nil, err
		}
		confirmed, err := s.ConfirmTOTP(ctx, token, code)
		if err != nil {
			return nil, err
		}
		if _, err := s.AckRecovery(ctx, confirmed.PendingToken, true); err != nil {
			return nil, err
		}
		return s.SkipPasskey(ctx, confirmed.PendingToken)
	case StageSecondFactor:
		pend, err := s.lookupPending(ctx, token)
		if err != nil {
			return nil, err
		}
		row, err := s.twoFA.GetTOTP(ctx, pend.AdminID)
		if err != nil {
			return nil, err
		}
		code, err := totp.GenerateCode(row.Secret, time.Now())
		if err != nil {
			return nil, err
		}
		return s.VerifyTOTP(ctx, token, code)
	default:
		return nil, ErrInvalidStage
	}
}

// Logout 只删除当前完整会话键。
func (s *AdminService) Logout(ctx context.Context, accessToken string) error {
	if s.cache == nil || strings.TrimSpace(accessToken) == "" {
		return nil
	}
	return s.cache.Delete(ctx, cache.AdminSessKey(accessToken))
}

// ParseToken 查找完整会话并滑动续期。被删除的账号视为令牌失效。
func (s *AdminService) ParseToken(ctx context.Context, tokenString string) (*model.Admin, error) {
	if s.cache == nil {
		return nil, ErrInvalidToken
	}
	raw, ok, err := s.cache.Get(ctx, cache.AdminSessKey(tokenString))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrInvalidToken
	}
	var payload sessionPayload
	if err := json.Unmarshal(raw, &payload); err != nil || payload.AdminID == uuid.Nil {
		return nil, ErrInvalidToken
	}
	admin, err := s.store.GetByID(ctx, payload.AdminID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrInvalidToken
	}
	if err != nil {
		return nil, err
	}
	_ = s.cache.SetWithTTL(ctx, cache.AdminSessKey(tokenString), raw, s.idle)
	return admin, nil
}

// ParsePendingToken 查找受限 token，不续期。
func (s *AdminService) ParsePendingToken(ctx context.Context, tokenString string) (*model.Admin, string, error) {
	pend, err := s.lookupPending(ctx, tokenString)
	if err != nil {
		return nil, "", err
	}
	admin, err := s.store.GetByID(ctx, pend.AdminID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, "", ErrInvalidToken
	}
	if err != nil {
		return nil, "", err
	}
	return admin, pend.Stage, nil
}

// SessionIdleSeconds 返回完整会话空闲 TTL（秒），供登录响应 expires_in。
func (s *AdminService) SessionIdleSeconds() int64 {
	return int64(s.idle / time.Second)
}

// PendingTTLSeconds 返回受限 token TTL（秒）。
func (s *AdminService) PendingTTLSeconds() int64 {
	return int64(s.pendingTTL / time.Second)
}

type sessionPayload struct {
	AdminID  uuid.UUID `json:"admin_id"`
	Username string    `json:"username"`
}

type pendingPayload struct {
	AdminID         uuid.UUID       `json:"admin_id"`
	Username        string          `json:"username"`
	Stage           string          `json:"stage"`
	ClientIP        string          `json:"client_ip"`
	ExpiresAt       time.Time       `json:"expires_at"`
	WebAuthnSession json.RawMessage `json:"webauthn_session,omitempty"`
}

func (s *AdminService) issuePending(ctx context.Context, admin *model.Admin, stage, clientIP string) (*LoginResult, error) {
	if s.cache == nil {
		return nil, ErrCacheUnavailable
	}
	token, err := newOpaqueToken()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	payload := pendingPayload{
		AdminID:     admin.ID,
		Username:    admin.Username,
		Stage:       stage,
		ClientIP:    clientIP,
		ExpiresAt:   now.Add(s.pendingTTL),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if err := s.cache.SetWithTTL(ctx, cache.AdminPendKey(token), raw, s.pendingTTL); err != nil {
		return nil, err
	}
	return &LoginResult{
		Status:       LoginStatusPending,
		PendingToken: token,
		Stage:        stage,
		ExpiresIn:    int64(s.pendingTTL / time.Second),
		Admin:        admin,
	}, nil
}

func (s *AdminService) issueSession(ctx context.Context, admin *model.Admin, clientIP, pendingToken string) (*LoginResult, error) {
	if s.cache == nil {
		return nil, ErrCacheUnavailable
	}
	now := time.Now().UTC()
	ip := lastLoginIPPtr(clientIP)
	if err := s.store.UpdateLastLogin(ctx, admin.ID, now, ip); err != nil {
		return nil, err
	}
	admin.LastLoginAt = &now
	admin.LastLoginIP = ip
	token, err := newOpaqueToken()
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(sessionPayload{AdminID: admin.ID, Username: admin.Username})
	if err != nil {
		return nil, err
	}
	if err := s.cache.SetWithTTL(ctx, cache.AdminSessKey(token), raw, s.idle); err != nil {
		return nil, err
	}
	if pendingToken != "" {
		_ = s.cache.Delete(ctx, cache.AdminPendKey(pendingToken))
	}
	fresh, err := s.store.GetByID(ctx, admin.ID)
	if err == nil {
		admin = fresh
	}
	return &LoginResult{
		Status:    LoginStatusComplete,
		Token:     token,
		ExpiresIn: int64(s.idle / time.Second),
		Admin:     admin,
	}, nil
}

func (s *AdminService) lookupPending(ctx context.Context, token string) (*pendingPayload, error) {
	if s.cache == nil || strings.TrimSpace(token) == "" {
		return nil, ErrInvalidToken
	}
	raw, ok, err := s.cache.Get(ctx, cache.AdminPendKey(token))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrInvalidToken
	}
	var payload pendingPayload
	if err := json.Unmarshal(raw, &payload); err != nil || payload.AdminID == uuid.Nil {
		return nil, ErrInvalidToken
	}
	if !payload.ExpiresAt.IsZero() && !time.Now().Before(payload.ExpiresAt) {
		_ = s.cache.Delete(ctx, cache.AdminPendKey(token))
		return nil, ErrInvalidToken
	}
	return &payload, nil
}

func (s *AdminService) savePending(ctx context.Context, token string, payload *pendingPayload) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	ttl := time.Until(payload.ExpiresAt)
	if ttl <= 0 {
		return ErrInvalidToken
	}
	return s.cache.SetWithTTL(ctx, cache.AdminPendKey(token), raw, ttl)
}

func (s *AdminService) pendingResult(token string, payload *pendingPayload, admin *model.Admin) *LoginResult {
	ttl := time.Until(payload.ExpiresAt)
	if ttl < 0 {
		ttl = 0
	}
	return &LoginResult{
		Status:       LoginStatusPending,
		PendingToken: token,
		Stage:        payload.Stage,
		ExpiresIn:    int64(ttl / time.Second),
		Admin:        admin,
	}
}

func lastLoginIPPtr(clientIP string) *string {
	ip := strings.TrimSpace(clientIP)
	if ip == "" {
		return nil
	}
	if len(ip) > lastLoginIPMaxLen {
		ip = ip[:lastLoginIPMaxLen]
	}
	return &ip
}

func newOpaqueToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func totpPeriodIndex(now time.Time) int64 {
	return now.Unix() / totpPeriodSeconds
}

func generateRecoveryCodes() ([]string, error) {
	codes := make([]string, recoveryCodeCount)
	buf := make([]byte, recoveryCodeChars)
	for i := 0; i < recoveryCodeCount; i++ {
		if _, err := rand.Read(buf); err != nil {
			return nil, err
		}
		raw := make([]byte, recoveryCodeChars)
		for j := 0; j < recoveryCodeChars; j++ {
			raw[j] = crockfordAlphabet[int(buf[j])%len(crockfordAlphabet)]
		}
		codes[i] = string(raw[:4]) + "-" + string(raw[4:])
	}
	return codes, nil
}

func normalizeRecovery(code string) string {
	s := strings.ToUpper(strings.TrimSpace(code))
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, " ", "")
	return s
}

func hashRecovery(normalized string) string {
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

func validateUsername(username string) error {
	if !usernameRE.MatchString(username) {
		return ErrInvalidUsername
	}
	return nil
}

func validatePassword(password string) error {
	if len(password) < minPasswordLen {
		return ErrWeakPassword
	}
	return nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate") || strings.Contains(msg, "unique")
}
