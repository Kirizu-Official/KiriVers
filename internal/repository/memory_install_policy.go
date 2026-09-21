package repository

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// MemoryInstallPolicyRuleStore 是进程内 InstallPolicyRuleStore，供单测使用。
type MemoryInstallPolicyRuleStore struct {
	mu   sync.Mutex
	rows map[uuid.UUID]*model.InstallPolicyRule
}

// NewMemoryInstallPolicyRuleStore 构造空的内存仓储。
func NewMemoryInstallPolicyRuleStore() *MemoryInstallPolicyRuleStore {
	return &MemoryInstallPolicyRuleStore{rows: map[uuid.UUID]*model.InstallPolicyRule{}}
}

func cloneInstallPolicyRule(src *model.InstallPolicyRule) *model.InstallPolicyRule {
	if src == nil {
		return nil
	}
	cp := *src
	return &cp
}

func installPolicyScopeMatch(row *model.InstallPolicyRule, projectID, channelID uuid.UUID, os, arch string) bool {
	return row.ProjectID == projectID && row.ChannelID == channelID && row.OS == os && row.Arch == arch
}

// List 列出某作用域 + 平台的规则，按 path 升序。
func (m *MemoryInstallPolicyRuleStore) List(_ context.Context, projectID, channelID uuid.UUID, os, arch string) ([]model.InstallPolicyRule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]model.InstallPolicyRule, 0)
	for _, row := range m.rows {
		if installPolicyScopeMatch(row, projectID, channelID, os, arch) {
			out = append(out, *cloneInstallPolicyRule(row))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// Replace 整表替换该作用域 + 平台的规则。
func (m *MemoryInstallPolicyRuleStore) Replace(_ context.Context, projectID, channelID uuid.UUID, os, arch string, rules []model.InstallPolicyRule) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, row := range m.rows {
		if installPolicyScopeMatch(row, projectID, channelID, os, arch) {
			delete(m.rows, id)
		}
	}
	now := time.Now().UTC()
	for i := range rules {
		row := rules[i]
		row.ProjectID = projectID
		row.ChannelID = channelID
		row.OS = os
		row.Arch = arch
		if err := row.BeforeCreate(nil); err != nil {
			return err
		}
		if row.CreatedAt.IsZero() {
			row.CreatedAt = now
		}
		row.UpdatedAt = now
		m.rows[row.ID] = cloneInstallPolicyRule(&row)
	}
	return nil
}

// DeleteByChannel 删除某自定义渠道的全部规则。
func (m *MemoryInstallPolicyRuleStore) DeleteByChannel(_ context.Context, projectID, channelID uuid.UUID) error {
	if channelID == uuid.Nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, row := range m.rows {
		if row.ProjectID == projectID && row.ChannelID == channelID {
			delete(m.rows, id)
		}
	}
	return nil
}
