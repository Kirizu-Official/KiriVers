package service

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/cache"
	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// PasskeyPublic 是安全页与列表返回的 Passkey 公开字段。
type PasskeyPublic struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// SetupTOTP 在 enroll_totp 阶段生成 pending 密钥。完整会话上的轮换走 SetupTOTPRotate。
func (s *AdminService) SetupTOTP(ctx context.Context, pendingToken string) (*LoginResult, error) {
	pend, err := s.lookupPending(ctx, pendingToken)
	if err != nil {
		return nil, err
	}
	if pend.Stage != StageEnrollTOTP {
		return nil, ErrInvalidStage
	}
	admin, err := s.Get(ctx, pend.AdminID)
	if err != nil {
		return nil, err
	}
	row, err := s.ensureTOTPRow(ctx, admin.ID)
	if err != nil {
		return nil, err
	}
	if row.Confirmed() {
		return nil, ErrInvalidStage
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      totpIssuer,
		AccountName: admin.Username,
		Period:      totpPeriodSeconds,
	})
	if err != nil {
		return nil, err
	}
	row.PendingSecret = key.Secret()
	if err := s.twoFA.UpsertTOTP(ctx, row); err != nil {
		return nil, err
	}
	out := s.pendingResult(pendingToken, pend, admin)
	out.Secret = key.Secret()
	out.OTPAuthURL = key.URL()
	return out, nil
}

// ConfirmTOTP 用当前代码确认 pending 密钥并生成恢复码，进入 ack_recovery。
// 此时尚未标记合格 2FA（ConfirmedAt 仍为空）；未 ack 就过期的话下次登录须重新绑定。
func (s *AdminService) ConfirmTOTP(ctx context.Context, pendingToken, code string) (*LoginResult, error) {
	pend, err := s.lookupPending(ctx, pendingToken)
	if err != nil {
		return nil, err
	}
	if pend.Stage != StageEnrollTOTP {
		return nil, ErrInvalidStage
	}
	admin, err := s.Get(ctx, pend.AdminID)
	if err != nil {
		return nil, err
	}
	row, err := s.twoFA.GetTOTP(ctx, admin.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	if row.PendingSecret == "" {
		return nil, ErrInvalidCredentials
	}
	if err := s.checkAndBumpTOTPFail(ctx, row, row.PendingSecret, code); err != nil {
		return nil, err
	}
	row.Secret = row.PendingSecret
	row.PendingSecret = ""
	row.ConfirmedAt = nil
	row.FailCount = 0
	if err := s.twoFA.UpsertTOTP(ctx, row); err != nil {
		return nil, err
	}
	plain, err := s.regenerateRecovery(ctx, admin.ID)
	if err != nil {
		return nil, err
	}
	pend.Stage = StageAckRecovery
	if err := s.savePending(ctx, pendingToken, pend); err != nil {
		return nil, err
	}
	out := s.pendingResult(pendingToken, pend, admin)
	out.RecoveryCodes = plain
	return out, nil
}

// AckRecovery 确认已保存恢复码，写入 ConfirmedAt，进入 optional_passkey。
func (s *AdminService) AckRecovery(ctx context.Context, pendingToken string, confirmed bool) (*LoginResult, error) {
	pend, err := s.lookupPending(ctx, pendingToken)
	if err != nil {
		return nil, err
	}
	if pend.Stage != StageAckRecovery {
		return nil, ErrInvalidStage
	}
	if !confirmed {
		return nil, ErrRecoveryNotAcked
	}
	admin, err := s.Get(ctx, pend.AdminID)
	if err != nil {
		return nil, err
	}
	row, err := s.twoFA.GetTOTP(ctx, admin.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTOTPNotConfirmed
		}
		return nil, err
	}
	if row.Secret == "" {
		return nil, ErrTOTPNotConfirmed
	}
	now := time.Now().UTC()
	row.ConfirmedAt = &now
	if err := s.twoFA.UpsertTOTP(ctx, row); err != nil {
		return nil, err
	}
	pend.Stage = StageOptionalPasskey
	if err := s.savePending(ctx, pendingToken, pend); err != nil {
		return nil, err
	}
	return s.pendingResult(pendingToken, pend, admin), nil
}

// SkipPasskey 在 optional_passkey 阶段签发完整会话。
func (s *AdminService) SkipPasskey(ctx context.Context, pendingToken string) (*LoginResult, error) {
	pend, err := s.lookupPending(ctx, pendingToken)
	if err != nil {
		return nil, err
	}
	if pend.Stage != StageOptionalPasskey {
		return nil, ErrInvalidStage
	}
	admin, err := s.Get(ctx, pend.AdminID)
	if err != nil {
		return nil, err
	}
	row, err := s.twoFA.GetTOTP(ctx, admin.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTOTPNotConfirmed
		}
		return nil, err
	}
	if !row.Confirmed() {
		return nil, ErrTOTPNotConfirmed
	}
	return s.issueSession(ctx, admin, pend.ClientIP, pendingToken)
}

// VerifyTOTP 以 TOTP 完成 second_factor。
func (s *AdminService) VerifyTOTP(ctx context.Context, pendingToken, code string) (*LoginResult, error) {
	pend, err := s.lookupPending(ctx, pendingToken)
	if err != nil {
		return nil, err
	}
	if pend.Stage != StageSecondFactor {
		return nil, ErrInvalidStage
	}
	admin, err := s.Get(ctx, pend.AdminID)
	if err != nil {
		return nil, err
	}
	row, err := s.twoFA.GetTOTP(ctx, admin.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	if !row.Confirmed() {
		return nil, ErrTOTPNotConfirmed
	}
	if err := s.checkAndBumpTOTPFail(ctx, row, row.Secret, code); err != nil {
		return nil, err
	}
	return s.issueSession(ctx, admin, pend.ClientIP, pendingToken)
}

// VerifyRecovery 以一次性恢复码完成 second_factor。
func (s *AdminService) VerifyRecovery(ctx context.Context, pendingToken, code string) (*LoginResult, error) {
	pend, err := s.lookupPending(ctx, pendingToken)
	if err != nil {
		return nil, err
	}
	if pend.Stage != StageSecondFactor {
		return nil, ErrInvalidStage
	}
	admin, err := s.Get(ctx, pend.AdminID)
	if err != nil {
		return nil, err
	}
	row, err := s.ensureTOTPRow(ctx, admin.ID)
	if err != nil {
		return nil, err
	}
	if !row.Confirmed() {
		return nil, ErrTOTPNotConfirmed
	}
	now := time.Now().UTC()
	period := totpPeriodIndex(now)
	if row.RecoveryFailPeriod != period {
		row.RecoveryFailCount = 0
		row.RecoveryFailPeriod = period
	}
	if row.RecoveryFailCount >= s.totpMax {
		_ = s.twoFA.UpsertTOTP(ctx, row)
		return nil, ErrTOTPRateLimited
	}
	normalized := normalizeRecovery(code)
	if normalized == "" {
		row.RecoveryFailCount++
		_ = s.twoFA.UpsertTOTP(ctx, row)
		return nil, ErrInvalidCredentials
	}
	want := hashRecovery(normalized)
	list, err := s.twoFA.ListRecoveryCodes(ctx, admin.ID)
	if err != nil {
		return nil, err
	}
	var matched *model.AdminRecoveryCode
	for i := range list {
		c := &list[i]
		if c.UsedAt != nil {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(c.CodeHash), []byte(want)) == 1 {
			matched = c
			break
		}
	}
	if matched == nil {
		row.RecoveryFailCount++
		_ = s.twoFA.UpsertTOTP(ctx, row)
		return nil, ErrInvalidCredentials
	}
	if err := s.twoFA.MarkRecoveryUsed(ctx, matched.ID, now); err != nil {
		return nil, err
	}
	row.RecoveryFailCount = 0
	_ = s.twoFA.UpsertTOTP(ctx, row)
	return s.issueSession(ctx, admin, pend.ClientIP, pendingToken)
}

// SetupTOTPRotate 在完整会话下写入 pending 密钥（不覆盖已确认密钥）。
func (s *AdminService) SetupTOTPRotate(ctx context.Context, adminID uuid.UUID) (*LoginResult, error) {
	admin, err := s.Get(ctx, adminID)
	if err != nil {
		return nil, err
	}
	row, err := s.twoFA.GetTOTP(ctx, adminID)
	if err != nil {
		return nil, err
	}
	if !row.Confirmed() {
		return nil, ErrTOTPNotConfirmed
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      totpIssuer,
		AccountName: admin.Username,
		Period:      totpPeriodSeconds,
	})
	if err != nil {
		return nil, err
	}
	row.PendingSecret = key.Secret()
	if err := s.twoFA.UpsertTOTP(ctx, row); err != nil {
		return nil, err
	}
	return &LoginResult{Secret: key.Secret(), OTPAuthURL: key.URL(), Admin: admin}, nil
}

// ConfirmTOTPRotate 确认轮换。不踢已有会话。
func (s *AdminService) ConfirmTOTPRotate(ctx context.Context, adminID uuid.UUID, code string) error {
	row, err := s.twoFA.GetTOTP(ctx, adminID)
	if err != nil {
		return err
	}
	if row.PendingSecret == "" {
		return ErrInvalidCredentials
	}
	if err := s.checkAndBumpTOTPFail(ctx, row, row.PendingSecret, code); err != nil {
		return err
	}
	now := time.Now().UTC()
	row.Secret = row.PendingSecret
	row.PendingSecret = ""
	row.ConfirmedAt = &now
	row.FailCount = 0
	return s.twoFA.UpsertTOTP(ctx, row)
}

// TwoFAStatus 返回安全页摘要。
func (s *AdminService) TwoFAStatus(ctx context.Context, adminID uuid.UUID) (*LoginResult, error) {
	admin, err := s.Get(ctx, adminID)
	if err != nil {
		return nil, err
	}
	out := &LoginResult{Admin: admin}
	if row, err := s.twoFA.GetTOTP(ctx, adminID); err == nil {
		out.TOTPEnabled = row.Confirmed()
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	codes, err := s.twoFA.ListRecoveryCodes(ctx, adminID)
	if err != nil {
		return nil, err
	}
	left := 0
	for _, c := range codes {
		if c.UsedAt == nil {
			left++
		}
	}
	out.RecoveryLeft = left
	passkeys, err := s.ListPasskeys(ctx, adminID)
	if err != nil {
		return nil, err
	}
	out.Passkeys = passkeys
	return out, nil
}

// RegenerateRecovery 作废未用旧码并返回新明文一次。
func (s *AdminService) RegenerateRecovery(ctx context.Context, adminID uuid.UUID) ([]string, error) {
	row, err := s.twoFA.GetTOTP(ctx, adminID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTOTPNotConfirmed
		}
		return nil, err
	}
	if !row.Confirmed() {
		return nil, ErrTOTPNotConfirmed
	}
	return s.regenerateRecovery(ctx, adminID)
}

// ListPasskeys 返回公开 Passkey 列表。
func (s *AdminService) ListPasskeys(ctx context.Context, adminID uuid.UUID) ([]PasskeyPublic, error) {
	list, err := s.twoFA.ListPasskeys(ctx, adminID)
	if err != nil {
		return nil, err
	}
	out := make([]PasskeyPublic, 0, len(list))
	for _, p := range list {
		out = append(out, PasskeyPublic{ID: p.ID, Name: p.Name, CreatedAt: p.CreatedAt})
	}
	return out, nil
}

// DeletePasskey 删除一把 Passkey，不踢会话。
func (s *AdminService) DeletePasskey(ctx context.Context, adminID, id uuid.UUID) error {
	if err := s.twoFA.DeletePasskey(ctx, adminID, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPasskeyNotFound
		}
		return err
	}
	return nil
}

// BeginPasskeyRegister 开始注册。pending 阶段仅允许 optional_passkey；完整会话要求已确认 TOTP。
func (s *AdminService) BeginPasskeyRegister(ctx context.Context, adminID uuid.UUID, pendingToken string) (json.RawMessage, error) {
	if err := s.requireWebAuthn(); err != nil {
		return nil, err
	}
	if pendingToken != "" {
		pend, err := s.lookupPending(ctx, pendingToken)
		if err != nil {
			return nil, err
		}
		if pend.Stage != StageOptionalPasskey || pend.AdminID != adminID {
			return nil, ErrInvalidStage
		}
	}
	row, err := s.twoFA.GetTOTP(ctx, adminID)
	if err != nil || !row.Confirmed() {
		return nil, ErrTOTPNotConfirmed
	}
	user, err := s.webAuthnUser(ctx, adminID)
	if err != nil {
		return nil, err
	}
	wa, err := s.webAuthnAPI()
	if err != nil {
		return nil, err
	}
	creation, session, err := wa.BeginRegistration(user)
	if err != nil {
		return nil, err
	}
	sessRaw, err := json.Marshal(session)
	if err != nil {
		return nil, err
	}
	if pendingToken != "" {
		pend, err := s.lookupPending(ctx, pendingToken)
		if err != nil {
			return nil, err
		}
		pend.WebAuthnSession = sessRaw
		if err := s.savePending(ctx, pendingToken, pend); err != nil {
			return nil, err
		}
	} else if err := s.saveFullCeremony(ctx, adminID, sessRaw); err != nil {
		return nil, err
	}
	return json.Marshal(creation)
}

// FinishPasskeyRegister 完成注册。pending optional_passkey 成功后签发完整会话。
func (s *AdminService) FinishPasskeyRegister(ctx context.Context, adminID uuid.UUID, pendingToken, name string, credential json.RawMessage) (*LoginResult, error) {
	if err := s.requireWebAuthn(); err != nil {
		return nil, err
	}
	var session webauthn.SessionData
	if pendingToken != "" {
		pend, err := s.lookupPending(ctx, pendingToken)
		if err != nil {
			return nil, err
		}
		if pend.Stage != StageOptionalPasskey || pend.AdminID != adminID {
			return nil, ErrInvalidStage
		}
		if len(pend.WebAuthnSession) == 0 {
			return nil, ErrInvalidRequest("missing webauthn session")
		}
		if err := json.Unmarshal(pend.WebAuthnSession, &session); err != nil {
			return nil, err
		}
	} else {
		raw, err := s.loadFullCeremony(ctx, adminID)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &session); err != nil {
			return nil, err
		}
	}
	user, err := s.webAuthnUser(ctx, adminID)
	if err != nil {
		return nil, err
	}
	wa, err := s.webAuthnAPI()
	if err != nil {
		return nil, err
	}
	parsed, err := protocol.ParseCredentialCreationResponseBody(bytes.NewReader(credential))
	if err != nil {
		return nil, ErrInvalidRequest("invalid webauthn credential")
	}
	cred, err := wa.CreateCredential(user, session, parsed)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	display := name
	if display == "" {
		display = "Passkey"
	}
	row := &model.AdminPasskey{
		AdminID:      adminID,
		CredentialID: cred.ID,
		PublicKey:    cred.PublicKey,
		SignCount:    cred.Authenticator.SignCount,
		Name:         display,
	}
	if err := s.twoFA.CreatePasskey(ctx, row); err != nil {
		return nil, err
	}
	if pendingToken != "" {
		admin, err := s.Get(ctx, adminID)
		if err != nil {
			return nil, err
		}
		pend, err := s.lookupPending(ctx, pendingToken)
		if err != nil {
			return nil, err
		}
		return s.issueSession(ctx, admin, pend.ClientIP, pendingToken)
	}
	return &LoginResult{}, nil
}

// BeginPasskeyLogin 开始 second_factor 断言。
func (s *AdminService) BeginPasskeyLogin(ctx context.Context, pendingToken string) (json.RawMessage, error) {
	if err := s.requireWebAuthn(); err != nil {
		return nil, err
	}
	pend, err := s.lookupPending(ctx, pendingToken)
	if err != nil {
		return nil, err
	}
	if pend.Stage != StageSecondFactor {
		return nil, ErrInvalidStage
	}
	user, err := s.webAuthnUser(ctx, pend.AdminID)
	if err != nil {
		return nil, err
	}
	if len(user.WebAuthnCredentials()) == 0 {
		return nil, ErrInvalidRequest("no passkeys registered")
	}
	wa, err := s.webAuthnAPI()
	if err != nil {
		return nil, err
	}
	assertion, session, err := wa.BeginLogin(user)
	if err != nil {
		return nil, err
	}
	sessRaw, err := json.Marshal(session)
	if err != nil {
		return nil, err
	}
	pend.WebAuthnSession = sessRaw
	if err := s.savePending(ctx, pendingToken, pend); err != nil {
		return nil, err
	}
	return json.Marshal(assertion)
}

// FinishPasskeyLogin 完成断言并签发完整会话。不受 TOTP 周期锁定。
func (s *AdminService) FinishPasskeyLogin(ctx context.Context, pendingToken string, credential json.RawMessage) (*LoginResult, error) {
	if err := s.requireWebAuthn(); err != nil {
		return nil, err
	}
	pend, err := s.lookupPending(ctx, pendingToken)
	if err != nil {
		return nil, err
	}
	if pend.Stage != StageSecondFactor {
		return nil, ErrInvalidStage
	}
	if len(pend.WebAuthnSession) == 0 {
		return nil, ErrInvalidRequest("missing webauthn session")
	}
	var session webauthn.SessionData
	if err := json.Unmarshal(pend.WebAuthnSession, &session); err != nil {
		return nil, err
	}
	user, err := s.webAuthnUser(ctx, pend.AdminID)
	if err != nil {
		return nil, err
	}
	wa, err := s.webAuthnAPI()
	if err != nil {
		return nil, err
	}
	parsed, err := protocol.ParseCredentialRequestResponseBody(bytes.NewReader(credential))
	if err != nil {
		return nil, ErrInvalidRequest("invalid webauthn assertion")
	}
	cred, err := wa.ValidateLogin(user, session, parsed)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	stored, err := s.twoFA.GetPasskeyByCredentialID(ctx, cred.ID)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	stored.SignCount = cred.Authenticator.SignCount
	if err := s.twoFA.UpdatePasskey(ctx, stored); err != nil {
		return nil, err
	}
	admin, err := s.Get(ctx, pend.AdminID)
	if err != nil {
		return nil, err
	}
	return s.issueSession(ctx, admin, pend.ClientIP, pendingToken)
}

func ErrInvalidRequest(msg string) error {
	return invalidRequestError{msg}
}

type invalidRequestError struct{ msg string }

func (e invalidRequestError) Error() string { return e.msg }

func IsInvalidRequest(err error) bool {
	var ir invalidRequestError
	return errors.As(err, &ir)
}

func (s *AdminService) checkAndBumpTOTPFail(ctx context.Context, row *model.AdminTOTP, secret, code string) error {
	now := time.Now()
	period := totpPeriodIndex(now)
	if row.FailPeriod != period {
		row.FailCount = 0
		row.FailPeriod = period
	}
	if row.FailCount >= s.totpMax {
		_ = s.twoFA.UpsertTOTP(ctx, row)
		return ErrTOTPRateLimited
	}
	ok, err := totp.ValidateCustom(code, secret, now, totp.ValidateOpts{
		Period:    uint(totpPeriodSeconds),
		Skew:      1,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	if err != nil || !ok {
		row.FailCount++
		_ = s.twoFA.UpsertTOTP(ctx, row)
		return ErrInvalidCredentials
	}
	row.FailCount = 0
	return s.twoFA.UpsertTOTP(ctx, row)
}

func (s *AdminService) ensureTOTPRow(ctx context.Context, adminID uuid.UUID) (*model.AdminTOTP, error) {
	row, err := s.twoFA.GetTOTP(ctx, adminID)
	if err == nil {
		return row, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	row = &model.AdminTOTP{AdminID: adminID}
	if err := s.twoFA.UpsertTOTP(ctx, row); err != nil {
		return nil, err
	}
	return row, nil
}

func (s *AdminService) regenerateRecovery(ctx context.Context, adminID uuid.UUID) ([]string, error) {
	plain, err := generateRecoveryCodes()
	if err != nil {
		return nil, err
	}
	rows := make([]model.AdminRecoveryCode, 0, len(plain))
	for _, code := range plain {
		rows = append(rows, model.AdminRecoveryCode{
			AdminID:  adminID,
			CodeHash: hashRecovery(normalizeRecovery(code)),
		})
	}
	if err := s.twoFA.ReplaceRecoveryCodes(ctx, adminID, rows); err != nil {
		return nil, err
	}
	return plain, nil
}

func (s *AdminService) requireWebAuthn() error {
	if s.webauthnRP == "" || len(s.webauthnOrg) == 0 {
		return ErrWebAuthnNotReady
	}
	return nil
}

func (s *AdminService) webAuthnAPI() (*webauthn.WebAuthn, error) {
	if err := s.requireWebAuthn(); err != nil {
		return nil, err
	}
	return webauthn.New(&webauthn.Config{
		RPDisplayName: totpIssuer,
		RPID:          s.webauthnRP,
		RPOrigins:     s.webauthnOrg,
	})
}

type webAuthnUser struct {
	id          []byte
	name        string
	displayName string
	creds       []webauthn.Credential
}

func (u webAuthnUser) WebAuthnID() []byte                         { return u.id }
func (u webAuthnUser) WebAuthnName() string                       { return u.name }
func (u webAuthnUser) WebAuthnDisplayName() string                { return u.displayName }
func (u webAuthnUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

func (s *AdminService) webAuthnUser(ctx context.Context, adminID uuid.UUID) (webAuthnUser, error) {
	admin, err := s.Get(ctx, adminID)
	if err != nil {
		return webAuthnUser{}, err
	}
	list, err := s.twoFA.ListPasskeys(ctx, adminID)
	if err != nil {
		return webAuthnUser{}, err
	}
	creds := make([]webauthn.Credential, 0, len(list))
	for _, p := range list {
		creds = append(creds, webauthn.Credential{
			ID:        p.CredentialID,
			PublicKey: p.PublicKey,
			Authenticator: webauthn.Authenticator{
				SignCount: p.SignCount,
			},
		})
	}
	id := make([]byte, 16)
	copy(id, admin.ID[:])
	return webAuthnUser{
		id:          id,
		name:        admin.Username,
		displayName: admin.Username,
		creds:       creds,
	}, nil
}

func (s *AdminService) saveFullCeremony(ctx context.Context, adminID uuid.UUID, raw json.RawMessage) error {
	if s.cache == nil {
		return ErrCacheUnavailable
	}
	return s.cache.SetWithTTL(ctx, cache.AdminWebAuthnKey(adminID), raw, s.pendingTTL)
}

func (s *AdminService) loadFullCeremony(ctx context.Context, adminID uuid.UUID) (json.RawMessage, error) {
	if s.cache == nil {
		return nil, ErrCacheUnavailable
	}
	raw, ok, err := s.cache.Get(ctx, cache.AdminWebAuthnKey(adminID))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrInvalidRequest("missing webauthn session")
	}
	return raw, nil
}
