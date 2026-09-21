package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// InstallPolicyRuleStore 负责 install_policy_rules。查询一律带 project_id。
// channelID 为 uuid.Nil 表示项目作用域。
type InstallPolicyRuleStore interface {
	List(ctx context.Context, projectID, channelID uuid.UUID, os, arch string) ([]model.InstallPolicyRule, error)
	Replace(ctx context.Context, projectID, channelID uuid.UUID, os, arch string, rules []model.InstallPolicyRule) error
	DeleteByChannel(ctx context.Context, projectID, channelID uuid.UUID) error
}

// InstallPolicyRuleRepo 是 PostgreSQL 实现。
type InstallPolicyRuleRepo struct {
	db *gorm.DB
}

// NewInstallPolicyRuleRepo 构造仓储。
func NewInstallPolicyRuleRepo(db *gorm.DB) *InstallPolicyRuleRepo {
	return &InstallPolicyRuleRepo{db: db}
}

// List 列出某作用域 + 平台的规则，按 path 升序。
func (r *InstallPolicyRuleRepo) List(ctx context.Context, projectID, channelID uuid.UUID, os, arch string) ([]model.InstallPolicyRule, error) {
	var rows []model.InstallPolicyRule
	err := r.db.WithContext(ctx).
		Where("project_id = ? AND channel_id = ? AND os = ? AND arch = ?", projectID, channelID, os, arch).
		Order("path ASC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list install policy rules: %w", err)
	}
	return rows, nil
}

// Replace 整表替换该作用域 + 平台的规则（事务：先删后插）。
func (r *InstallPolicyRuleRepo) Replace(ctx context.Context, projectID, channelID uuid.UUID, os, arch string, rules []model.InstallPolicyRule) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("project_id = ? AND channel_id = ? AND os = ? AND arch = ?", projectID, channelID, os, arch).
			Delete(&model.InstallPolicyRule{}).Error; err != nil {
			return fmt.Errorf("delete install policy rules: %w", err)
		}
		if len(rules) == 0 {
			return nil
		}
		if err := tx.Create(&rules).Error; err != nil {
			return fmt.Errorf("insert install policy rules: %w", err)
		}
		return nil
	})
}

// DeleteByChannel 删除某自定义渠道的全部规则（渠道 DELETE 时调用）。
func (r *InstallPolicyRuleRepo) DeleteByChannel(ctx context.Context, projectID, channelID uuid.UUID) error {
	if channelID == uuid.Nil {
		return nil
	}
	if err := r.db.WithContext(ctx).
		Where("project_id = ? AND channel_id = ?", projectID, channelID).
		Delete(&model.InstallPolicyRule{}).Error; err != nil {
		return fmt.Errorf("delete install policy rules by channel: %w", err)
	}
	return nil
}
