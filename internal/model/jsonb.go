package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// StringList 是 PostgreSQL jsonb 字符串数组（例如 CORS origins）。
type StringList []string

// GormDataType 告诉 GORM 使用 jsonb。
func (StringList) GormDataType() string { return "jsonb" }

// Value 实现 driver.Valuer。
func (s StringList) Value() (driver.Value, error) {
	if s == nil {
		return []byte("[]"), nil
	}
	b, err := json.Marshal([]string(s))
	if err != nil {
		return nil, err
	}
	return b, nil
}

// Scan 实现 sql.Scanner。
func (s *StringList) Scan(value any) error {
	if s == nil {
		return fmt.Errorf("StringList: nil receiver")
	}
	if value == nil {
		*s = StringList{}
		return nil
	}
	raw, err := jsonbBytes(value)
	if err != nil {
		return err
	}
	var list []string
	if len(raw) == 0 {
		*s = StringList{}
		return nil
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return err
	}
	*s = list
	return nil
}

// JSONObject 是 jsonb 对象袋（限流默认、商店协议等）。
type JSONObject map[string]any

// GormDataType 告诉 GORM 使用 jsonb。
func (JSONObject) GormDataType() string { return "jsonb" }

// Value 实现 driver.Valuer。
func (o JSONObject) Value() (driver.Value, error) {
	if o == nil {
		return []byte("{}"), nil
	}
	b, err := json.Marshal(map[string]any(o))
	if err != nil {
		return nil, err
	}
	return b, nil
}

// Scan 实现 sql.Scanner。
func (o *JSONObject) Scan(value any) error {
	if o == nil {
		return fmt.Errorf("JSONObject: nil receiver")
	}
	if value == nil {
		*o = JSONObject{}
		return nil
	}
	raw, err := jsonbBytes(value)
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		*o = JSONObject{}
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}
	*o = m
	return nil
}

// IdentifierMap 是商店 listing 的协议包名袋（bundle id、PackageIdentifier 等）。
type IdentifierMap map[string]string

// GormDataType 告诉 GORM 使用 jsonb。
func (IdentifierMap) GormDataType() string { return "jsonb" }

// Value 实现 driver.Valuer。
func (m IdentifierMap) Value() (driver.Value, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	b, err := json.Marshal(map[string]string(m))
	if err != nil {
		return nil, err
	}
	return b, nil
}

// Scan 实现 sql.Scanner。
func (m *IdentifierMap) Scan(value any) error {
	if m == nil {
		return fmt.Errorf("IdentifierMap: nil receiver")
	}
	if value == nil {
		*m = IdentifierMap{}
		return nil
	}
	raw, err := jsonbBytes(value)
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		*m = IdentifierMap{}
		return nil
	}
	var parsed map[string]string
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return err
	}
	*m = parsed
	return nil
}

func jsonbBytes(value any) ([]byte, error) {
	switch v := value.(type) {
	case []byte:
		return v, nil
	case string:
		return []byte(v), nil
	default:
		return nil, fmt.Errorf("unsupported jsonb type %T", value)
	}
}

// DefaultRateLimit 返回文档建议的项目级限流袋（§14）；实际 429 由
// internal/middleware 的滑动窗口 Limiter 执行，键常量见 project.go。
func DefaultRateLimit() JSONObject {
	return JSONObject{
		"check_per_device_per_minute":     60,
		"check_per_ip_per_minute":         120,
		"store_per_ip_per_minute":          300,
		"diff_per_device_per_minute":      20,
		"diff_per_ip_per_minute":          60,
		"telemetry_per_device_per_minute": 30,
		"ci_per_token_per_minute":         600,
	}
}

// ChangelogEntry 是某一语言下的版本更新日志项，包含可选标题与 Markdown 正文。
type ChangelogEntry struct {
	Title    string `json:"title,omitempty"`
	Markdown string `json:"markdown"`
}

// ChangelogMap 是以语言代码（如 en, zh-CN）为键的多语言更新日志 jsonb 字典。
type ChangelogMap map[string]ChangelogEntry

// GormDataType 告诉 GORM 使用 jsonb。
func (ChangelogMap) GormDataType() string { return "jsonb" }

// Value 实现 driver.Valuer。
func (m ChangelogMap) Value() (driver.Value, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	b, err := json.Marshal(map[string]ChangelogEntry(m))
	if err != nil {
		return nil, err
	}
	return b, nil
}

// Scan 实现 sql.Scanner。
func (m *ChangelogMap) Scan(value any) error {
	if m == nil {
		return fmt.Errorf("ChangelogMap: nil receiver")
	}
	if value == nil {
		*m = ChangelogMap{}
		return nil
	}
	raw, err := jsonbBytes(value)
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		*m = ChangelogMap{}
		return nil
	}
	var res map[string]ChangelogEntry
	if err := json.Unmarshal(raw, &res); err != nil {
		return err
	}
	*m = res
	return nil
}

