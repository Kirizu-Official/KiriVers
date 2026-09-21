package database

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

func testPostgresDSN() string {
	if dsn := os.Getenv("KIRIVERS_POSTGRES_DSN"); dsn != "" {
		return dsn
	}
	return "postgres://kirivers:8WdeOilkFYo6OcBE3bdmg2@localhost:5432/kirivers?sslmode=disable"
}

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := testPostgresDSN()
	db, err := Open(dsn, zerolog.Nop())
	if err != nil {
		t.Skipf("skipping postgres test: cannot open database: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := Ping(ctx, db); err != nil {
		t.Skipf("skipping postgres test: ping failed: %v", err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}
	return db
}

type indexInfo struct {
	TableName string `gorm:"column:tablename"`
	IndexName string `gorm:"column:indexname"`
	IndexDef  string `gorm:"column:indexdef"`
}

func loadTableIndexes(t *testing.T, db *gorm.DB) map[string]map[string]string {
	t.Helper()
	var rows []indexInfo
	err := db.Raw(`SELECT tablename, indexname, indexdef FROM pg_indexes WHERE schemaname = current_schema()`).Scan(&rows).Error
	if err != nil {
		t.Fatalf("query pg_indexes: %v", err)
	}
	m := make(map[string]map[string]string)
	for _, r := range rows {
		if m[r.TableName] == nil {
			m[r.TableName] = make(map[string]string)
		}
		m[r.TableName][r.IndexName] = r.IndexDef
	}
	return m
}

func TestPostgresSchemaIndexes(t *testing.T) {
	db := openTestDB(t)

	// AC7: 验证 AutoMigrate 幂等（连续再次运行无错误）
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("subsequent AutoMigrate must be idempotent: %v", err)
	}

	idxMap := loadTableIndexes(t, db)

	t.Run("AC1_JobsIndexes", func(t *testing.T) {
		jobsIdx := idxMap["jobs"]
		if jobsIdx == nil {
			t.Fatal("jobs table has no indexes")
		}
		claimDef, ok := jobsIdx["idx_jobs_claim"]
		if !ok {
			t.Fatal("expected idx_jobs_claim to exist")
		}
		if !strings.Contains(claimDef, "WHERE") || !strings.Contains(claimDef, "status = 'queued'") {
			t.Fatalf("idx_jobs_claim must be partial WHERE status = 'queued', got: %s", claimDef)
		}
		idemDef, ok := jobsIdx["idx_jobs_idempotency"]
		if !ok {
			t.Fatal("expected idx_jobs_idempotency to exist")
		}
		if strings.Contains(idemDef, "UNIQUE") {
			t.Fatalf("idx_jobs_idempotency must NOT be UNIQUE, got: %s", idemDef)
		}
		if _, exists := jobsIdx["idx_jobs_idempotency_key"]; exists {
			t.Fatal("old single-column idx_jobs_idempotency_key must be dropped")
		}
	})

	t.Run("AC2_ArtifactsIndexes", func(t *testing.T) {
		artIdx := idxMap["artifacts"]
		if artIdx == nil {
			t.Fatal("artifacts table has no indexes")
		}
		for _, required := range []string{
			"idx_artifacts_line_kind",
			"idx_artifacts_project_sha256",
			"idx_artifacts_project_file_name",
		} {
			if _, ok := artIdx[required]; !ok {
				t.Fatalf("expected artifact composite index %s to exist", required)
			}
		}
		for _, dropped := range []string{
			"idx_artifacts_file_name",
			"idx_artifacts_sha256",
			"idx_artifacts_fileset_sha256",
			"idx_artifacts_version_line_id",
			"idx_artifacts_project_id",
		} {
			if _, exists := artIdx[dropped]; exists {
				t.Fatalf("old redundant artifact index %s must be dropped", dropped)
			}
		}
	})

	t.Run("AC3_VersionsIndexes", func(t *testing.T) {
		vIdx := idxMap["versions"]
		if vIdx == nil {
			t.Fatal("versions table has no indexes")
		}
		if _, ok := vIdx["idx_versions_project_status"]; !ok {
			t.Fatal("expected idx_versions_project_status to exist")
		}
		for _, dropped := range []string{
			"idx_versions_channel_id",
			"idx_versions_channel_slug",
			"idx_versions_channel",
			"idx_versions_status",
			"idx_versions_project_id",
			"idx_versions_version_integer",
			"idx_versions_version_semver_canonical",
		} {
			if _, exists := vIdx[dropped]; exists {
				t.Fatalf("old unused version index %s must be dropped", dropped)
			}
		}
	})

	t.Run("AC4_UploadSessionsIndexes", func(t *testing.T) {
		upIdx := idxMap["upload_sessions"]
		if upIdx == nil {
			t.Fatal("upload_sessions table has no indexes")
		}
		if _, ok := upIdx["idx_upload_sessions_line_status"]; !ok {
			t.Fatal("expected idx_upload_sessions_line_status to exist")
		}
		idemDef, ok := upIdx["idx_upload_sessions_project_idem"]
		if !ok {
			t.Fatal("expected idx_upload_sessions_project_idem to exist")
		}
		if strings.Contains(idemDef, "UNIQUE") {
			t.Fatalf("idx_upload_sessions_project_idem must NOT be UNIQUE, got: %s", idemDef)
		}
		for _, dropped := range []string{
			"idx_upload_sessions_status",
			"idx_upload_sessions_version_line_id",
			"idx_upload_sessions_idempotency_key",
		} {
			if _, exists := upIdx[dropped]; exists {
				t.Fatalf("old upload_sessions index %s must be dropped", dropped)
			}
		}
	})

	t.Run("AC5_NodeAndLanguageUniqueIndexes", func(t *testing.T) {
		nodeSyncIdx := idxMap["node_artifact_sync"]
		if nodeSyncIdx == nil {
			t.Fatal("node_artifact_sync table has no indexes")
		}
		nodeDef, ok := nodeSyncIdx["idx_node_artifact_sync_scope"]
		if !ok {
			t.Fatal("expected idx_node_artifact_sync_scope to exist")
		}
		if !strings.Contains(nodeDef, "UNIQUE") || !strings.Contains(nodeDef, "COALESCE") {
			t.Fatalf("idx_node_artifact_sync_scope must be UNIQUE with COALESCE, got: %s", nodeDef)
		}

		langIdx := idxMap["project_languages"]
		if langIdx == nil {
			t.Fatal("project_languages table has no indexes")
		}
		langDef, ok := langIdx["idx_project_languages_one_default"]
		if !ok {
			t.Fatal("expected idx_project_languages_one_default to exist")
		}
		if !strings.Contains(langDef, "UNIQUE") || !strings.Contains(langDef, "is_default") {
			t.Fatalf("idx_project_languages_one_default must be UNIQUE WHERE is_default, got: %s", langDef)
		}
	})

	t.Run("AC6_ClientsIndexes", func(t *testing.T) {
		cliIdx := idxMap["clients"]
		if cliIdx == nil {
			t.Fatal("clients table has no indexes")
		}
		for _, required := range []string{
			"idx_clients_project_device",
			"idx_clients_project_os",
			"idx_clients_project_arch",
			"idx_clients_project_version",
			"idx_clients_project_country",
			"idx_clients_custom_jsonb",
			"idx_clients_custom_search",
			"idx_clients_last_version_trgm",
			"idx_clients_last_ip_trgm",
			"idx_clients_last_os_trgm",
			"idx_clients_last_arch_trgm",
			"idx_clients_last_channel_trgm",
		} {
			if _, ok := cliIdx[required]; !ok {
				t.Fatalf("expected client index %s to exist", required)
			}
		}
	})

	t.Run("GrayAndTelemetryIndexes", func(t *testing.T) {
		grayIdx := idxMap["gray_allowlist"]
		if grayIdx == nil {
			t.Fatal("gray_allowlist table has no indexes")
		}
		if _, ok := grayIdx["idx_gray_allowlist_project_device"]; !ok {
			t.Fatal("expected idx_gray_allowlist_project_device to exist")
		}
		if _, ok := grayIdx["idx_gray_allowlist_version_device"]; !ok {
			t.Fatal("expected idx_gray_allowlist_version_device to exist")
		}
		if _, exists := grayIdx["idx_gray_allowlist_project_id"]; exists {
			t.Fatal("old single-column idx_gray_allowlist_project_id must be dropped")
		}

		telemIdx := idxMap["telemetry_events"]
		if telemIdx == nil {
			t.Fatal("telemetry_events table has no indexes")
		}
		if _, ok := telemIdx["idx_telemetry_privacy"]; !ok {
			t.Fatal("expected idx_telemetry_privacy to exist")
		}
	})
}

func TestPostgresUniqueConstraints(t *testing.T) {
	db := openTestDB(t)

	t.Run("TwoProjectsCanShareSemVer", func(t *testing.T) {
		tx := db.Begin()
		defer tx.Rollback()

		p1 := model.Project{ID: uuid.New(), Slug: "test-p1-" + uuid.New().String()[:8], Name: "P1"}
		p2 := model.Project{ID: uuid.New(), Slug: "test-p2-" + uuid.New().String()[:8], Name: "P2"}
		if err := tx.Create(&p1).Error; err != nil {
			t.Fatalf("create p1: %v", err)
		}
		if err := tx.Create(&p2).Error; err != nil {
			t.Fatalf("create p2: %v", err)
		}

		canonical := "1.0.0"
		semver := "v1.0.0"
		v1 := model.Version{
			ID:                     uuid.New(),
			ProjectID:              p1.ID,
			ChannelSlug:            "stable",
			VersionSemver:          &semver,
			VersionSemverCanonical: &canonical,
			Status:                 model.VersionStatusDraft,
		}
		v2 := model.Version{
			ID:                     uuid.New(),
			ProjectID:              p2.ID,
			ChannelSlug:            "stable",
			VersionSemver:          &semver,
			VersionSemverCanonical: &canonical,
			Status:                 model.VersionStatusDraft,
		}
		if err := tx.Create(&v1).Error; err != nil {
			t.Fatalf("insert version for p1 failed: %v", err)
		}
		if err := tx.Create(&v2).Error; err != nil {
			t.Fatalf("insert same semver for p2 must succeed across projects, but got: %v", err)
		}

		// 同项目内重复 semver 必须失败
		v3 := model.Version{
			ID:                     uuid.New(),
			ProjectID:              p1.ID,
			ChannelSlug:            "stable",
			VersionSemver:          &semver,
			VersionSemverCanonical: &canonical,
			Status:                 model.VersionStatusDraft,
		}
		if err := tx.Create(&v3).Error; err == nil {
			t.Fatal("insert duplicate semver within same project must fail")
		}
	})

	t.Run("JobsIdempotencyAllowsDuplicateKey", func(t *testing.T) {
		tx := db.Begin()
		defer tx.Rollback()

		pid := uuid.New()
		p := model.Project{ID: pid, Slug: "test-idem-" + uuid.New().String()[:8], Name: "Idem"}
		if err := tx.Create(&p).Error; err != nil {
			t.Fatalf("create project: %v", err)
		}

		key := "test-idempotency-key"
		j1 := model.Job{
			ID:             uuid.New(),
			Type:           "bundle_unpack",
			Status:         model.JobStatusQueued,
			ProjectID:      &pid,
			IdempotencyKey: &key,
		}
		j2 := model.Job{
			ID:             uuid.New(),
			Type:           "bundle_unpack",
			Status:         model.JobStatusQueued,
			ProjectID:      &pid,
			IdempotencyKey: &key,
		}
		if err := tx.Create(&j1).Error; err != nil {
			t.Fatalf("create job 1: %v", err)
		}
		if err := tx.Create(&j2).Error; err != nil {
			t.Fatalf("create job 2 with same idempotency key must succeed (24h window, not UNIQUE): %v", err)
		}
	})

	t.Run("NodeArtifactSyncPreventsDuplicateNullLine", func(t *testing.T) {
		tx := db.Begin()
		defer tx.Rollback()

		nodeID := uuid.New()
		versionID := uuid.New()
		projectID := uuid.New()

		s1 := model.NodeArtifactSync{
			ID:        uuid.New(),
			NodeID:    nodeID,
			ProjectID: projectID,
			VersionID: versionID,
			LineID:    nil,
			Status:    model.NodeSyncStatusReady,
		}
		if err := tx.Create(&s1).Error; err != nil {
			t.Fatalf("first node_artifact_sync with NULL line_id failed: %v", err)
		}

		s2 := model.NodeArtifactSync{
			ID:        uuid.New(),
			NodeID:    nodeID,
			ProjectID: projectID,
			VersionID: versionID,
			LineID:    nil,
			Status:    model.NodeSyncStatusSyncing,
		}
		if err := tx.Create(&s2).Error; err == nil {
			t.Fatal("second node_artifact_sync with NULL line_id must fail unique constraint")
		}
	})

	t.Run("ProjectLanguagesPreventsDuplicateDefault", func(t *testing.T) {
		tx := db.Begin()
		defer tx.Rollback()

		pid := uuid.New()
		p := model.Project{ID: pid, Slug: "test-lang-" + uuid.New().String()[:8], Name: "Lang"}
		if err := tx.Create(&p).Error; err != nil {
			t.Fatalf("create project: %v", err)
		}

		l1 := model.ProjectLanguage{
			ID:        uuid.New(),
			ProjectID: pid,
			Code:      "en",
			IsDefault: true,
			SortOrder: 0,
		}
		if err := tx.Create(&l1).Error; err != nil {
			t.Fatalf("create default language: %v", err)
		}

		l2 := model.ProjectLanguage{
			ID:        uuid.New(),
			ProjectID: pid,
			Code:      "zh-CN",
			IsDefault: true,
			SortOrder: 1,
		}
		if err := tx.Create(&l2).Error; err == nil {
			t.Fatal("second default language must fail unique constraint")
		}
	})
}
