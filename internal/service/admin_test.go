package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"

	"github.com/Kirizu-Official/KiriVers/internal/cache"
	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
)

func mustMemoryCache(t *testing.T) cache.Store {
	t.Helper()
	store, err := cache.Open(cache.Options{Driver: "memory"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestAdminCreateLoginAndLastDelete(t *testing.T) {
	ctx := context.Background()
	svc := NewTestAdminService(repository.NewMemoryAdminStore())

	if _, err := svc.Create(ctx, "ab", "password123"); !errors.Is(err, ErrInvalidUsername) {
		t.Fatalf("username: %v", err)
	}
	if _, err := svc.Create(ctx, "root", "short"); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("password: %v", err)
	}

	a, err := svc.Create(ctx, "root", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, "root", "password123"); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("dup: %v", err)
	}

	pending, err := svc.Login(ctx, "root", "password123", "")
	if err != nil {
		t.Fatal(err)
	}
	if pending.Status != LoginStatusPending || pending.Token != "" || pending.PendingToken == "" {
		t.Fatalf("password login must be pending: %+v", pending)
	}
	if _, err := svc.ParseToken(ctx, pending.PendingToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("pending token must not parse as session: %v", err)
	}

	login, err := svc.CompleteLogin(ctx, "root", "password123", "")
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.ParseToken(ctx, login.Token)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != a.ID {
		t.Fatalf("id mismatch")
	}
	if _, err := svc.Login(ctx, "root", "wrong-password", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("bad login: %v", err)
	}

	if err := svc.Delete(ctx, a.ID); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("last admin: %v", err)
	}

	if _, err := svc.Create(ctx, "ops", "password123"); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteByUsername(ctx, "ops"); err != nil {
		t.Fatal(err)
	}
	if err := svc.ResetPassword(ctx, "root", "newpass123"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CompleteLogin(ctx, "root", "newpass123", ""); err != nil {
		t.Fatal(err)
	}
}

type failLastLoginStore struct {
	*repository.MemoryAdminStore
}

func (f *failLastLoginStore) UpdateLastLogin(context.Context, uuid.UUID, time.Time, *string) error {
	return errors.New("persist failed")
}

func TestAdminLoginDoesNotWriteLastLoginUntilSession(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryAdminStore()
	svc := NewTestAdminService(store)
	a, err := svc.Create(ctx, "root", "password123")
	if err != nil {
		t.Fatal(err)
	}

	pending, err := svc.Login(ctx, "root", "password123", "192.0.2.1")
	if err != nil {
		t.Fatal(err)
	}
	if pending.Status != LoginStatusPending {
		t.Fatal("expected pending")
	}
	stored, err := store.GetByID(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.LastLoginAt != nil || stored.LastLoginIP != nil {
		t.Fatalf("password step must not write last login: at=%v ip=%v", stored.LastLoginAt, stored.LastLoginIP)
	}

	before := time.Now().UTC().Add(-time.Second)
	login, err := svc.CompleteLogin(ctx, "root", "password123", "192.0.2.1")
	if err != nil {
		t.Fatal(err)
	}
	after := time.Now().UTC().Add(time.Second)
	if login.Token == "" {
		t.Fatal("expected token after successful persist")
	}
	if login.Admin.LastLoginIP == nil || *login.Admin.LastLoginIP != "192.0.2.1" {
		t.Fatalf("login response ip=%v", login.Admin.LastLoginIP)
	}
	if login.Admin.LastLoginAt == nil {
		t.Fatal("login response time is nil")
	}
	gotAt := login.Admin.LastLoginAt.UTC()
	if gotAt.Before(before) || gotAt.After(after) {
		t.Fatalf("last_login_at=%v not in [%v,%v]", gotAt, before, after)
	}

	if _, err := svc.Login(ctx, "root", "wrong-password", "203.0.113.9"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("bad login: %v", err)
	}
	stored, err = store.GetByID(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.LastLoginIP == nil || *stored.LastLoginIP != "192.0.2.1" {
		t.Fatalf("failed login overwrote ip: %v", stored.LastLoginIP)
	}
	firstAt := *stored.LastLoginAt

	second, err := svc.CompleteLogin(ctx, "root", "password123", "198.51.100.1")
	if err != nil {
		t.Fatal(err)
	}
	if second.Admin.LastLoginIP == nil || *second.Admin.LastLoginIP != "198.51.100.1" {
		t.Fatalf("second login ip=%v", second.Admin.LastLoginIP)
	}
	if second.Admin.LastLoginAt == nil || second.Admin.LastLoginAt.Before(firstAt) {
		t.Fatalf("second login time=%v first=%v", second.Admin.LastLoginAt, firstAt)
	}

	empty, err := svc.CompleteLogin(ctx, "root", "password123", "   ")
	if err != nil {
		t.Fatal(err)
	}
	if empty.Admin.LastLoginIP != nil {
		t.Fatalf("empty ip must persist nil, got %v", empty.Admin.LastLoginIP)
	}

	long := strings.Repeat("a", 80)
	trunc, err := svc.CompleteLogin(ctx, "root", "password123", long)
	if err != nil {
		t.Fatal(err)
	}
	if trunc.Admin.LastLoginIP == nil || len(*trunc.Admin.LastLoginIP) != lastLoginIPMaxLen {
		t.Fatalf("truncated ip len=%v", trunc.Admin.LastLoginIP)
	}
}

func TestAdminLoginPersistFailureDoesNotIssueToken(t *testing.T) {
	ctx := context.Background()
	store := &failLastLoginStore{MemoryAdminStore: repository.NewMemoryAdminStore()}
	svc := NewTestAdminService(store)
	if _, err := svc.Create(ctx, "root", "password123"); err != nil {
		t.Fatal(err)
	}
	out, err := svc.CompleteLogin(ctx, "root", "password123", "192.0.2.1")
	if err == nil || out != nil {
		t.Fatalf("persist failure must return error without token, out=%v err=%v", out, err)
	}
	got, err := store.GetByUsername(ctx, "root")
	if err != nil {
		t.Fatal(err)
	}
	if got.LastLoginAt != nil || got.LastLoginIP != nil {
		t.Fatalf("failed persist must not write last login: at=%v ip=%v", got.LastLoginAt, got.LastLoginIP)
	}
}

func TestAdminTOTPRateLimitAndRecovery(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryAdminStore()
	svc := NewAdminService(AdminServiceOptions{
		Store:           store,
		TwoFA:           store,
		Cache:           mustMemoryCache(t),
		TOTPMaxAttempts: 2,
		PendingTTL:      time.Minute,
		SessionIdle:     time.Hour,
	})
	if _, err := svc.Create(ctx, "root", "password123"); err != nil {
		t.Fatal(err)
	}
	full, err := svc.CompleteLogin(ctx, "root", "password123", "192.0.2.1")
	if err != nil {
		t.Fatal(err)
	}
	firstToken := full.Token

	pending, err := svc.Login(ctx, "root", "password123", "")
	if err != nil {
		t.Fatal(err)
	}
	if pending.Stage != StageSecondFactor {
		t.Fatalf("stage=%s", pending.Stage)
	}
	if _, err := svc.VerifyTOTP(ctx, pending.PendingToken, "000000"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("bad totp: %v", err)
	}
	if _, err := svc.VerifyTOTP(ctx, pending.PendingToken, "000000"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("second bad totp: %v", err)
	}
	if _, err := svc.VerifyTOTP(ctx, pending.PendingToken, "000000"); !errors.Is(err, ErrTOTPRateLimited) {
		t.Fatalf("rate limit: %v", err)
	}

	if _, err := svc.ParseToken(ctx, firstToken); err != nil {
		t.Fatalf("existing session must survive failed 2FA: %v", err)
	}

	// 下一时间步应可再验。
	row, err := store.GetTOTP(ctx, full.Admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	row.FailPeriod = totpPeriodIndex(time.Now()) - 1
	row.FailCount = 2
	if err := store.UpsertTOTP(ctx, row); err != nil {
		t.Fatal(err)
	}
	code, err := totp.GenerateCode(row.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	again, err := svc.Login(ctx, "root", "password123", "")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := svc.VerifyTOTP(ctx, again.PendingToken, code)
	if err != nil {
		t.Fatal(err)
	}
	if ok.Token == "" || ok.Token == firstToken {
		t.Fatal("second session must be a new token")
	}
	if err := svc.Logout(ctx, firstToken); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ParseToken(ctx, firstToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("logged out session: %v", err)
	}
	if _, err := svc.ParseToken(ctx, ok.Token); err != nil {
		t.Fatalf("second session after logout first: %v", err)
	}
}

func TestAdminRecoveryOnceAndClear2FAKeepsSession(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryAdminStore()
	svc := NewTestAdminService(store)
	if _, err := svc.Create(ctx, "root", "password123"); err != nil {
		t.Fatal(err)
	}
	pend, err := svc.Login(ctx, "root", "password123", "10.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	setup, err := svc.SetupTOTP(ctx, pend.PendingToken)
	if err != nil {
		t.Fatal(err)
	}
	code, err := totp.GenerateCode(setup.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	confirmed, err := svc.ConfirmTOTP(ctx, pend.PendingToken, code)
	if err != nil {
		t.Fatal(err)
	}
	if len(confirmed.RecoveryCodes) != recoveryCodeCount {
		t.Fatalf("recovery count=%d", len(confirmed.RecoveryCodes))
	}
	preAck, err := store.GetTOTP(ctx, confirmed.Admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	if preAck.Confirmed() {
		t.Fatal("totp must stay unconfirmed until recovery ack")
	}
	if _, err := svc.SkipPasskey(ctx, pend.PendingToken); !errors.Is(err, ErrInvalidStage) {
		t.Fatalf("skip before ack: %v", err)
	}
	if _, err := svc.AckRecovery(ctx, pend.PendingToken, false); !errors.Is(err, ErrRecoveryNotAcked) {
		t.Fatalf("ack false: %v", err)
	}
	if _, err := svc.AckRecovery(ctx, pend.PendingToken, true); err != nil {
		t.Fatal(err)
	}
	row, err := store.GetTOTP(ctx, confirmed.Admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !row.Confirmed() {
		t.Fatal("ack must mark totp confirmed")
	}
	sess, err := svc.SkipPasskey(ctx, pend.PendingToken)
	if err != nil {
		t.Fatal(err)
	}

	secondPend, err := svc.Login(ctx, "root", "password123", "")
	if err != nil {
		t.Fatal(err)
	}
	used := confirmed.RecoveryCodes[0]
	ok, err := svc.VerifyRecovery(ctx, secondPend.PendingToken, used)
	if err != nil {
		t.Fatal(err)
	}
	if ok.Token == "" {
		t.Fatal("recovery should issue session")
	}
	thirdPend, err := svc.Login(ctx, "root", "password123", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.VerifyRecovery(ctx, thirdPend.PendingToken, used); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("reuse recovery: %v", err)
	}

	fresh, err := svc.RegenerateRecovery(ctx, sess.Admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ParseToken(ctx, sess.Token); err != nil {
		t.Fatalf("regenerate recovery must not revoke sessions: %v", err)
	}
	fourth, err := svc.Login(ctx, "root", "password123", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.VerifyRecovery(ctx, fourth.PendingToken, used); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old recovery after regenerate: %v", err)
	}
	if _, err := svc.VerifyRecovery(ctx, fourth.PendingToken, fresh[0]); err != nil {
		t.Fatal(err)
	}

	if err := svc.Clear2FA(ctx, "root"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetTOTP(ctx, sess.Admin.ID); err == nil {
		t.Fatal("totp row should be gone")
	}
	if _, err := svc.ParseToken(ctx, sess.Token); err != nil {
		t.Fatalf("clear-2fa must not revoke sessions: %v", err)
	}
	rebind, err := svc.Login(ctx, "root", "password123", "")
	if err != nil {
		t.Fatal(err)
	}
	if rebind.Stage != StageEnrollTOTP {
		t.Fatalf("after clear-2fa stage=%s", rebind.Stage)
	}
}

func TestAdminPendingTTLAndSessionSlide(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryAdminStore()
	svc := NewAdminService(AdminServiceOptions{
		Store:       store,
		TwoFA:       store,
		Cache:       mustMemoryCache(t),
		PendingTTL:  80 * time.Millisecond,
		SessionIdle: 150 * time.Millisecond,
	})
	if _, err := svc.Create(ctx, "root", "password123"); err != nil {
		t.Fatal(err)
	}
	pend, err := svc.Login(ctx, "root", "password123", "")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(120 * time.Millisecond)
	if _, err := svc.SetupTOTP(ctx, pend.PendingToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expired pending: %v", err)
	}

	full, err := svc.CompleteLogin(ctx, "root", "password123", "")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(40 * time.Millisecond)
	if _, err := svc.ParseToken(ctx, full.Token); err != nil {
		t.Fatalf("slide should keep session: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if _, err := svc.ParseToken(ctx, full.Token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("idle expiry: %v", err)
	}
}

func TestAdminMustAckRecoveryBeforeRelogin(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryAdminStore()
	svc := NewTestAdminService(store)
	if _, err := svc.Create(ctx, "root", "password123"); err != nil {
		t.Fatal(err)
	}
	pend, err := svc.Login(ctx, "root", "password123", "10.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	setup, err := svc.SetupTOTP(ctx, pend.PendingToken)
	if err != nil {
		t.Fatal(err)
	}
	code, err := totp.GenerateCode(setup.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ConfirmTOTP(ctx, pend.PendingToken, code); err != nil {
		t.Fatal(err)
	}
	again, err := svc.Login(ctx, "root", "password123", "")
	if err != nil {
		t.Fatal(err)
	}
	if again.Stage != StageEnrollTOTP {
		t.Fatalf("unacked totp must re-enroll, stage=%s", again.Stage)
	}
	if _, err := svc.ParseToken(ctx, pend.PendingToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("pending must not be a session: %v", err)
	}
}

func TestAdminCachePayloadHasNoSecrets(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryAdminStore()
	mem := mustMemoryCache(t)
	svc := NewAdminService(AdminServiceOptions{
		Store: store,
		TwoFA: store,
		Cache: mem,
	})
	admin, err := svc.Create(ctx, "root", "password123")
	if err != nil {
		t.Fatal(err)
	}
	pend, err := svc.Login(ctx, "root", "password123", "192.0.2.1")
	if err != nil {
		t.Fatal(err)
	}
	raw, ok, err := mem.Get(ctx, cache.AdminPendKey(pend.PendingToken))
	if err != nil || !ok {
		t.Fatalf("pending cache miss: ok=%v err=%v", ok, err)
	}
	assertCacheJSONNoSecrets(t, raw)
	full, err := svc.CompleteLogin(ctx, "root", "password123", "192.0.2.1")
	if err != nil {
		t.Fatal(err)
	}
	sessRaw, ok, err := mem.Get(ctx, cache.AdminSessKey(full.Token))
	if err != nil || !ok {
		t.Fatalf("session cache miss: ok=%v err=%v", ok, err)
	}
	var payload map[string]any
	if err := json.Unmarshal(sessRaw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["admin_id"] != admin.ID.String() || payload["username"] != "root" {
		t.Fatalf("session payload=%v", payload)
	}
	assertCacheJSONNoSecrets(t, sessRaw)
}

func assertCacheJSONNoSecrets(t *testing.T, raw []byte) {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"secret", "pending_secret", "otpauth_url", "recovery_codes", "recovery", "private_key", "has_passkey"} {
		if _, ok := payload[key]; ok {
			t.Fatalf("cache JSON must not contain %q: %s", key, raw)
		}
	}
}

func TestAdminLoginHasPasskey(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryAdminStore()
	mem := mustMemoryCache(t)
	svc := NewAdminService(AdminServiceOptions{
		Store: store,
		TwoFA: store,
		Cache: mem,
	})
	admin, err := svc.Create(ctx, "root", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CompleteLogin(ctx, "root", "password123", ""); err != nil {
		t.Fatal(err)
	}

	none, err := svc.Login(ctx, "root", "password123", "")
	if err != nil {
		t.Fatal(err)
	}
	if none.Stage != StageSecondFactor {
		t.Fatalf("stage=%s", none.Stage)
	}
	if none.HasPasskey {
		t.Fatal("zero passkeys must yield HasPasskey false")
	}
	raw, ok, err := mem.Get(ctx, cache.AdminPendKey(none.PendingToken))
	if err != nil || !ok {
		t.Fatalf("pending cache miss: ok=%v err=%v", ok, err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if _, exists := payload["has_passkey"]; exists {
		t.Fatalf("pending cache must not store has_passkey: %s", raw)
	}

	if err := store.CreatePasskey(ctx, &model.AdminPasskey{
		AdminID:      admin.ID,
		CredentialID: []byte("cred-test-1"),
		PublicKey:    []byte("pubkey-test-1"),
		Name:         "laptop",
	}); err != nil {
		t.Fatal(err)
	}
	withKey, err := svc.Login(ctx, "root", "password123", "")
	if err != nil {
		t.Fatal(err)
	}
	if withKey.Stage != StageSecondFactor || !withKey.HasPasskey {
		t.Fatalf("registered passkey must yield HasPasskey true: stage=%s has=%v", withKey.Stage, withKey.HasPasskey)
	}
}
