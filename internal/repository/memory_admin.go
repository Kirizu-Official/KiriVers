package repository

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// MemoryAdminStore 是进程内实现，供单测使用，不用于生产 HTTP/CLI。
type MemoryAdminStore struct {
	mu       sync.Mutex
	byID     map[uuid.UUID]*model.Admin
	byName   map[string]uuid.UUID
	totp     map[uuid.UUID]*model.AdminTOTP
	passkeys map[uuid.UUID]*model.AdminPasskey
	recovery map[uuid.UUID]*model.AdminRecoveryCode
}

// NewMemoryAdminStore 构造空的内存仓储。
func NewMemoryAdminStore() *MemoryAdminStore {
	return &MemoryAdminStore{
		byID:     map[uuid.UUID]*model.Admin{},
		byName:   map[string]uuid.UUID{},
		totp:     map[uuid.UUID]*model.AdminTOTP{},
		passkeys: map[uuid.UUID]*model.AdminPasskey{},
		recovery: map[uuid.UUID]*model.AdminRecoveryCode{},
	}
}

func (m *MemoryAdminStore) Create(_ context.Context, admin *model.Admin) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if admin.ID == uuid.Nil {
		admin.ID = uuid.New()
	}
	if _, ok := m.byName[admin.Username]; ok {
		return gorm.ErrDuplicatedKey
	}
	cp := *admin
	m.byID[admin.ID] = &cp
	m.byName[admin.Username] = admin.ID
	return nil
}

func (m *MemoryAdminStore) GetByID(_ context.Context, id uuid.UUID) (*model.Admin, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.byID[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	cp := *a
	return &cp, nil
}

func (m *MemoryAdminStore) GetByUsername(_ context.Context, username string) (*model.Admin, error) {
	m.mu.Lock()
	id, ok := m.byName[username]
	m.mu.Unlock()
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return m.GetByID(context.Background(), id)
}

func (m *MemoryAdminStore) List(_ context.Context) ([]model.Admin, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]model.Admin, 0, len(m.byID))
	for _, a := range m.byID {
		out = append(out, *a)
	}
	return out, nil
}

func (m *MemoryAdminStore) Save(_ context.Context, admin *model.Admin) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	old, ok := m.byID[admin.ID]
	if !ok {
		return gorm.ErrRecordNotFound
	}
	if old.Username != admin.Username {
		delete(m.byName, old.Username)
		if _, exists := m.byName[admin.Username]; exists {
			return gorm.ErrDuplicatedKey
		}
		m.byName[admin.Username] = admin.ID
	}
	cp := *admin
	m.byID[admin.ID] = &cp
	return nil
}

// UpdateLastLogin 只改上次登录字段，不覆盖并发中的用户名/密码。
func (m *MemoryAdminStore) UpdateLastLogin(_ context.Context, id uuid.UUID, at time.Time, ip *string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.byID[id]
	if !ok {
		return gorm.ErrRecordNotFound
	}
	tcopy := at
	a.LastLoginAt = &tcopy
	if ip == nil {
		a.LastLoginIP = nil
	} else {
		scopy := *ip
		a.LastLoginIP = &scopy
	}
	a.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *MemoryAdminStore) Delete(_ context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.byID[id]
	if !ok {
		return gorm.ErrRecordNotFound
	}
	delete(m.byName, a.Username)
	delete(m.byID, id)
	return nil
}

func (m *MemoryAdminStore) Count(_ context.Context) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return int64(len(m.byID)), nil
}

func cloneBytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out
}

func (m *MemoryAdminStore) GetTOTP(_ context.Context, adminID uuid.UUID) (*model.AdminTOTP, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.totp[adminID]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	cp := *row
	return &cp, nil
}

func (m *MemoryAdminStore) UpsertTOTP(_ context.Context, row *model.AdminTOTP) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *row
	m.totp[row.AdminID] = &cp
	return nil
}

func (m *MemoryAdminStore) DeleteTOTP(_ context.Context, adminID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.totp, adminID)
	return nil
}

func (m *MemoryAdminStore) ListPasskeys(_ context.Context, adminID uuid.UUID) ([]model.AdminPasskey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]model.AdminPasskey, 0)
	for _, p := range m.passkeys {
		if p.AdminID == adminID {
			cp := *p
			cp.CredentialID = cloneBytes(p.CredentialID)
			cp.PublicKey = cloneBytes(p.PublicKey)
			out = append(out, cp)
		}
	}
	return out, nil
}

func (m *MemoryAdminStore) GetPasskey(_ context.Context, adminID, id uuid.UUID) (*model.AdminPasskey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.passkeys[id]
	if !ok || p.AdminID != adminID {
		return nil, gorm.ErrRecordNotFound
	}
	cp := *p
	cp.CredentialID = cloneBytes(p.CredentialID)
	cp.PublicKey = cloneBytes(p.PublicKey)
	return &cp, nil
}

func (m *MemoryAdminStore) GetPasskeyByCredentialID(_ context.Context, credentialID []byte) (*model.AdminPasskey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.passkeys {
		if string(p.CredentialID) == string(credentialID) {
			cp := *p
			cp.CredentialID = cloneBytes(p.CredentialID)
			cp.PublicKey = cloneBytes(p.PublicKey)
			return &cp, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (m *MemoryAdminStore) CreatePasskey(_ context.Context, row *model.AdminPasskey) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	if _, ok := m.passkeys[row.ID]; ok {
		return gorm.ErrDuplicatedKey
	}
	for _, p := range m.passkeys {
		if string(p.CredentialID) == string(row.CredentialID) {
			return gorm.ErrDuplicatedKey
		}
	}
	cp := *row
	cp.CredentialID = cloneBytes(row.CredentialID)
	cp.PublicKey = cloneBytes(row.PublicKey)
	m.passkeys[row.ID] = &cp
	return nil
}

func (m *MemoryAdminStore) UpdatePasskey(_ context.Context, row *model.AdminPasskey) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.passkeys[row.ID]; !ok {
		return gorm.ErrRecordNotFound
	}
	cp := *row
	cp.CredentialID = cloneBytes(row.CredentialID)
	cp.PublicKey = cloneBytes(row.PublicKey)
	m.passkeys[row.ID] = &cp
	return nil
}

func (m *MemoryAdminStore) DeletePasskey(_ context.Context, adminID, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.passkeys[id]
	if !ok || p.AdminID != adminID {
		return gorm.ErrRecordNotFound
	}
	delete(m.passkeys, id)
	return nil
}

func (m *MemoryAdminStore) DeletePasskeysByAdmin(_ context.Context, adminID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, p := range m.passkeys {
		if p.AdminID == adminID {
			delete(m.passkeys, id)
		}
	}
	return nil
}

func (m *MemoryAdminStore) ListRecoveryCodes(_ context.Context, adminID uuid.UUID) ([]model.AdminRecoveryCode, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]model.AdminRecoveryCode, 0)
	for _, c := range m.recovery {
		if c.AdminID == adminID {
			out = append(out, *c)
		}
	}
	return out, nil
}

func (m *MemoryAdminStore) ReplaceRecoveryCodes(_ context.Context, adminID uuid.UUID, codes []model.AdminRecoveryCode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, c := range m.recovery {
		if c.AdminID == adminID {
			delete(m.recovery, id)
		}
	}
	for i := range codes {
		row := codes[i]
		if row.ID == uuid.Nil {
			row.ID = uuid.New()
			codes[i].ID = row.ID
		}
		row.AdminID = adminID
		cp := row
		m.recovery[row.ID] = &cp
	}
	return nil
}

func (m *MemoryAdminStore) MarkRecoveryUsed(_ context.Context, id uuid.UUID, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.recovery[id]
	if !ok || c.UsedAt != nil {
		return gorm.ErrRecordNotFound
	}
	tcopy := at
	c.UsedAt = &tcopy
	return nil
}

func (m *MemoryAdminStore) DeleteRecoveryCodes(_ context.Context, adminID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, c := range m.recovery {
		if c.AdminID == adminID {
			delete(m.recovery, id)
		}
	}
	return nil
}
