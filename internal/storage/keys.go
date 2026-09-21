package storage

import (
	"net/url"
	"path"
	"strings"

	"github.com/google/uuid"
)

const (
	// NodeIdentityFileName 是 {local.root} 下的节点 UUID 文件。
	NodeIdentityFileName = ".kirivers-node-id"
)

// ArtifactObjectKey 返回公共产物正式键 {slug}/{sha256}（小写 64 hex，无 artifacts/ 前缀、无文件名段）。
func ArtifactObjectKey(slug, sha256 string) string {
	return strings.TrimSpace(slug) + "/" + strings.ToLower(strings.TrimSpace(sha256))
}

// ArtifactTempRel 返回仅落本机磁盘的上传暂存相对路径 {slug}/temp/{uuid}。不得写入公共桶。
func ArtifactTempRel(slug string, id uuid.UUID) string {
	return strings.TrimSpace(slug) + "/temp/" + id.String()
}

// GeoipTempRel 返回 GeoIP 本机暂存相对路径 geoip/temp/{uuid}。
func GeoipTempRel(id uuid.UUID) string {
	return "geoip/temp/" + id.String()
}

// GeoipObjectKey 返回私有桶键 geoip/{id}/{filename}。
func GeoipObjectKey(id uuid.UUID, fileName string) string {
	return "geoip/" + id.String() + "/" + strings.TrimSpace(fileName)
}

// PublicObjectURL 拼公共产物匿名 GET 绝对 URL（无预签名 query）。
// publicBaseURL 非空时作 CDN 前缀；否则按 endpoint+bucket 拼 path-style 或虚拟主机。
func PublicObjectURL(cfg S3Settings, key string) string {
	key = strings.TrimLeft(strings.TrimSpace(key), "/")
	base := strings.TrimRight(strings.TrimSpace(cfg.PublicBaseURL), "/")
	if base != "" {
		return base + "/" + key
	}
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		endpoint = "https://s3.amazonaws.com"
	}
	if !strings.Contains(endpoint, "://") {
		endpoint = "https://" + endpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return strings.TrimRight(endpoint, "/") + "/" + strings.TrimSpace(cfg.Bucket) + "/" + key
	}
	bucket := strings.TrimSpace(cfg.Bucket)
	if cfg.UsePathStyle || bucket == "" {
		u.Path = path.Join("/", bucket, key)
		return u.String()
	}
	// 虚拟主机：bucket.endpoint/key
	u.Host = bucket + "." + u.Host
	u.Path = path.Join("/", key)
	return u.String()
}
