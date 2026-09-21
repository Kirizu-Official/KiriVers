package database

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

func dropNamedColumns(m gorm.Migrator) error {
	type drop struct {
		model any
		col   string
	}
	drops := []drop{
		{&model.Project{}, "min_client_protocol"},
		{&model.Project{}, "default_rollout_percent"},
		{&model.Version{}, "required_intermediate_version"},
		{&model.Version{}, "rollout_percent"},
		{&model.Version{}, "gray_salt"},
		{&model.PlatformMatrix{}, "min_os"},
		{&model.PlatformMatrix{}, "min_api_level"},
		{&model.VersionLine{}, "rollout_percent_override"},
		{&model.GrayAllowlist{}, "version_line_id"},
		{&model.Project{}, "store_protocols"},
		{&model.Announcement{}, "version_ref"},
	}
	for _, d := range drops {
		if !m.HasTable(d.model) || !m.HasColumn(d.model, d.col) {
			continue
		}
		if err := m.DropColumn(d.model, d.col); err != nil {
			return fmt.Errorf("drop %s: %w", d.col, err)
		}
	}
	return nil
}

// recreateCompositeUniqueIndexes 重建必须含 project_id 的唯一索引，并去掉
// gray_allowlist 旧的 partial / per-line 索引。
func dropConflictingIndexes(db *gorm.DB) error {
	drops := []string{
		`DROP INDEX IF EXISTS idx_versions_project_integer`,
		`DROP INDEX IF EXISTS idx_versions_project_semver`,
		`DROP INDEX IF EXISTS idx_gray_allowlist_line_device`,
		`DROP INDEX IF EXISTS idx_gray_allowlist_version_device`,

		// jobs: 旧 claim 复合索引（由 partial WHERE 替换）与单列幂等键索引
		`DROP INDEX IF EXISTS idx_jobs_claim`,
		`DROP INDEX IF EXISTS idx_jobs_idempotency_key`,

		// artifacts: 旧单列索引（已由复合 line+kind、project+sha256、project+file_name 覆盖）
		`DROP INDEX IF EXISTS idx_artifacts_file_name`,
		`DROP INDEX IF EXISTS idx_artifacts_sha256`,
		`DROP INDEX IF EXISTS idx_artifacts_fileset_sha256`,
		`DROP INDEX IF EXISTS idx_artifacts_version_line_id`,
		`DROP INDEX IF EXISTS idx_artifacts_project_id`,

		// versions: 旧单列索引（已由复合 project+status 覆盖或未被查询使用）
		`DROP INDEX IF EXISTS idx_versions_channel_id`,
		`DROP INDEX IF EXISTS idx_versions_channel_slug`,
		`DROP INDEX IF EXISTS idx_versions_channel`,
		`DROP INDEX IF EXISTS idx_versions_status`,
		`DROP INDEX IF EXISTS idx_versions_project_id`,
		`DROP INDEX IF EXISTS idx_versions_version_integer`,
		`DROP INDEX IF EXISTS idx_versions_version_semver_canonical`,

		// version_lines: 旧单列 status 索引
		`DROP INDEX IF EXISTS idx_version_lines_status`,

		// upload_sessions: 旧单列索引
		`DROP INDEX IF EXISTS idx_upload_sessions_version_line_id`,
		`DROP INDEX IF EXISTS idx_upload_sessions_status`,
		`DROP INDEX IF EXISTS idx_upload_sessions_idempotency_key`,

		// gray_allowlist: 旧单列 project_id 索引（由复合 project+device 覆盖）
		`DROP INDEX IF EXISTS idx_gray_allowlist_project_id`,

		// node_artifact_sync: 旧 GORM unique 索引（由 COALESCE 表达式唯一索引替换）
		`DROP INDEX IF EXISTS idx_node_artifact_sync_scope`,
	}
	for _, sql := range drops {
		if err := db.Exec(sql).Error; err != nil {
			return fmt.Errorf("drop conflicting index: %w", err)
		}
	}
	return nil
}

func recreateCompositeUniqueIndexes(db *gorm.DB) error {
	stmts := []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_versions_project_integer ON versions (project_id, version_integer) WHERE version_integer IS NOT NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_versions_project_semver ON versions (project_id, version_semver_canonical) WHERE version_semver_canonical IS NOT NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_gray_allowlist_version_device ON gray_allowlist (version_id, device_id)`,
	}
	for _, sql := range stmts {
		if err := db.Exec(sql).Error; err != nil {
			return fmt.Errorf("recreate unique index: %w", err)
		}
	}
	return nil
}

// ensureHotPathIndexes 创建 GORM 无法直接表达的 partial / 表达式 / 业务单 default 索引。
func ensureHotPathIndexes(db *gorm.DB) error {
	stmts := []string{
		`CREATE INDEX IF NOT EXISTS idx_jobs_claim ON jobs (created_at ASC) WHERE status = 'queued'`,
		`CREATE INDEX IF NOT EXISTS idx_jobs_idempotency ON jobs (project_id, idempotency_key, created_at DESC) WHERE idempotency_key IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_upload_sessions_project_idem ON upload_sessions (project_id, idempotency_key) WHERE idempotency_key IS NOT NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_node_artifact_sync_scope ON node_artifact_sync (node_id, version_id, COALESCE(line_id, '00000000-0000-0000-0000-000000000000'::uuid))`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_project_languages_one_default ON project_languages (project_id) WHERE is_default`,
	}
	for _, sql := range stmts {
		if err := db.Exec(sql).Error; err != nil {
			return fmt.Errorf("ensure hot path index: %w", err)
		}
	}
	return nil
}

// alterLeftoverVarcharToText 把 information_schema 中残留的 varchar/character 列改为 text。
func alterLeftoverVarcharToText(db *gorm.DB) error {
	const sql = `
DO $$
DECLARE
  r RECORD;
BEGIN
  FOR r IN
    SELECT c.table_name, c.column_name
    FROM information_schema.columns c
    JOIN information_schema.tables t
      ON t.table_schema = c.table_schema AND t.table_name = c.table_name
    WHERE c.table_schema = current_schema()
      AND t.table_type = 'BASE TABLE'
      AND c.data_type IN ('character varying', 'character')
  LOOP
    EXECUTE format('ALTER TABLE %I ALTER COLUMN %I TYPE text', r.table_name, r.column_name);
  END LOOP;
END $$;`
	if err := db.Exec(sql).Error; err != nil {
		return fmt.Errorf("alter varchar to text: %w", err)
	}
	return nil
}

func ensureClientSearchIndexes(db *gorm.DB) error {
	if !db.Migrator().HasTable(&model.Client{}) {
		return nil
	}
	stmts := []string{
		`CREATE EXTENSION IF NOT EXISTS pg_trgm`,
		`CREATE INDEX IF NOT EXISTS idx_clients_custom_jsonb ON clients USING GIN (custom jsonb_path_ops)`,
		`CREATE INDEX IF NOT EXISTS idx_clients_custom_search ON clients USING GIN (to_tsvector('simple', COALESCE(custom::text, '')))`,
		`CREATE INDEX IF NOT EXISTS idx_clients_last_version_trgm ON clients USING GIN (last_version gin_trgm_ops)`,
		`CREATE INDEX IF NOT EXISTS idx_clients_last_ip_trgm ON clients USING GIN (last_ip gin_trgm_ops)`,
		`CREATE INDEX IF NOT EXISTS idx_clients_last_os_trgm ON clients USING GIN (last_os gin_trgm_ops)`,
		`CREATE INDEX IF NOT EXISTS idx_clients_last_arch_trgm ON clients USING GIN (last_arch gin_trgm_ops)`,
		`CREATE INDEX IF NOT EXISTS idx_clients_last_channel_trgm ON clients USING GIN (last_channel gin_trgm_ops)`,
	}
	for _, sql := range stmts {
		if err := db.Exec(sql).Error; err != nil {
			return fmt.Errorf("client search index: %w", err)
		}
	}
	return nil
}

func backfillPublishedGrayComplete(db *gorm.DB) error {
	if !db.Migrator().HasTable(&model.Version{}) || !db.Migrator().HasColumn(&model.Version{}, "gray_completed_at") {
		return nil
	}
	sql := `UPDATE versions SET gray_completed_at = COALESCE(publish_time, created_at)
WHERE gray_completed_at IS NULL AND status IN ('published','deprecated','revoked')`
	if err := db.Exec(sql).Error; err != nil {
		return fmt.Errorf("backfill gray_completed_at: %w", err)
	}
	return nil
}

func tableHasColumn(db *gorm.DB, table, col string) (bool, error) {
	var n int64
	err := db.Raw(`SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`, table, col).Scan(&n).Error
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func tableExists(db *gorm.DB, table string) (bool, error) {
	var n int64
	err := db.Raw(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = ?`, table).Scan(&n).Error
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// renameStoreSurface 在 AutoMigrate 之前把旧 feed_* 列/kind/jsonb 键改成 store_*。
// GORM 不会 RENAME：若先 AutoMigrate，会 ADD 空的 store_token_hash 并留下旧哈希。
func renameStoreSurface(db *gorm.DB) error {
	ok, err := tableExists(db, "projects")
	if err != nil {
		return fmt.Errorf("rename store surface: %w", err)
	}
	if ok {
		hasOld, err := tableHasColumn(db, "projects", "feed_token_hash")
		if err != nil {
			return fmt.Errorf("rename store surface: %w", err)
		}
		hasNew, err := tableHasColumn(db, "projects", "store_token_hash")
		if err != nil {
			return fmt.Errorf("rename store surface: %w", err)
		}
		if hasOld && !hasNew {
			if err := db.Exec(`ALTER TABLE projects RENAME COLUMN feed_token_hash TO store_token_hash`).Error; err != nil {
				return fmt.Errorf("rename feed_token_hash: %w", err)
			}
		}
		if err := db.Exec(`
UPDATE projects
SET rate_limit = CASE
  WHEN COALESCE(rate_limit, '{}'::jsonb) ? 'store_per_ip_per_minute'
    THEN COALESCE(rate_limit, '{}'::jsonb) - 'feed_per_ip_per_minute'
  ELSE (COALESCE(rate_limit, '{}'::jsonb) - 'feed_per_ip_per_minute')
    || jsonb_build_object('store_per_ip_per_minute', rate_limit->'feed_per_ip_per_minute')
END
WHERE COALESCE(rate_limit, '{}'::jsonb) ? 'feed_per_ip_per_minute'`).Error; err != nil {
			return fmt.Errorf("rewrite store_per_ip_per_minute: %w", err)
		}
	}
	ok, err = tableExists(db, "artifacts")
	if err != nil {
		return fmt.Errorf("rename store surface: %w", err)
	}
	if ok {
		if err := db.Exec(`UPDATE artifacts SET kind = 'store_full' WHERE kind = 'feed_full'`).Error; err != nil {
			return fmt.Errorf("rewrite store_full kind: %w", err)
		}
	}
	return nil
}
