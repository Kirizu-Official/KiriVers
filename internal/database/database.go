// Package database 负责 GORM + PostgreSQL 连接、健康探测与运行中 Watch 重连，不包含业务查询。
package database

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// Open 使用 DSN 打开连接。log 接收 GORM Warn/Error/慢查询（调用方注入 mod=db）。
// 调用方负责在进程退出时 Close。dsn 为空时失败，不回退。
func Open(dsn string, log zerolog.Logger) (*gorm.DB, error) {
	if dsn == "" {
		return nil, fmt.Errorf("postgres dsn is empty")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: newGormLogger(log),
	})
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	return db, nil
}

// Ping 探测底层 SQL 连接是否可用，供 /ready 使用。
func Ping(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is nil")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// Close 关闭连接池。
func Close(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// AutoMigrate 按 model.AutoMigrateModels 中的结构体同步表结构。
// GORM 只加列/加表。任务禁止兼容层时，随后对具名列执行 DropColumn。
// 新实体必须登记到 AutoMigrateModels。
func AutoMigrate(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is nil")
	}
	if err := renameStoreSurface(db); err != nil {
		return err
	}
	if err := renameAnnouncementJSONColumn(db); err != nil {
		return err
	}
	if err := db.AutoMigrate(model.AutoMigrateModels()...); err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}
	// 既有行的 name 默认为空串；回填为 slug，避免管理台标题空白。
	if err := db.Exec(`UPDATE projects SET name = slug WHERE name = ''`).Error; err != nil {
		return fmt.Errorf("backfill project names: %w", err)
	}
	// 具名 DropColumn：GORM AutoMigrate 只加列。产品未上线，禁止留下未使用列。
	m := db.Migrator()
	if err := dropConflictingIndexes(db); err != nil {
		return err
	}
	if err := dropNamedColumns(m); err != nil {
		return err
	}
	if err := expandAnnouncementJSON(db); err != nil {
		return err
	}
	if err := recreateCompositeUniqueIndexes(db); err != nil {
		return err
	}
	if err := ensureHotPathIndexes(db); err != nil {
		return err
	}
	if err := db.Exec(`UPDATE channels SET name = slug WHERE name = ''`).Error; err != nil {
		return fmt.Errorf("backfill channel names: %w", err)
	}
	if err := ensureProjectLanguageCIIndex(db); err != nil {
		return err
	}
	if err := BackfillProjectLanguages(db); err != nil {
		return fmt.Errorf("backfill project languages: %w", err)
	}
	if err := alterLeftoverVarcharToText(db); err != nil {
		return err
	}
	if err := ensureClientSearchIndexes(db); err != nil {
		return err
	}
	if err := backfillPublishedGrayComplete(db); err != nil {
		return err
	}
	return nil
}
