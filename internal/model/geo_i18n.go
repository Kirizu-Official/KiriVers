package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// GeoI18n 是融合后的多语言地名图，按字段分别落 locale→名称。
//
// 用途：客户端来源地只存 ISO 代码 + 全量地名图，不存单一语言的 country_name。
// 管理端按控制台 locale 选词。country / region 各自对每个 locale 键取融合链上第一份非空。
//
// 字段：
//   - Country：国家名 map，例如 {"zh-CN":"中国","en":"China"}。
//   - Region：一级行政区名 map，缺省为空对象。
type GeoI18n struct {
	Country map[string]string `json:"country,omitempty"`
	Region  map[string]string `json:"region,omitempty"`
}

// GormDataType 告诉 GORM 使用 jsonb。
func (GeoI18n) GormDataType() string { return "jsonb" }

// Value 实现 driver.Valuer。空值写成 {}。
func (g GeoI18n) Value() (driver.Value, error) {
	if g.Country == nil && g.Region == nil {
		return []byte("{}"), nil
	}
	b, err := json.Marshal(g)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// Scan 实现 sql.Scanner。
func (g *GeoI18n) Scan(value any) error {
	if g == nil {
		return fmt.Errorf("GeoI18n: nil receiver")
	}
	if value == nil {
		*g = GeoI18n{}
		return nil
	}
	raw, err := jsonbBytes(value)
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		*g = GeoI18n{}
		return nil
	}
	var parsed GeoI18n
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return err
	}
	*g = parsed
	return nil
}

// Clone 深拷贝地名图，避免 upsert 共享 map。
func (g GeoI18n) Clone() GeoI18n {
	out := GeoI18n{}
	if len(g.Country) > 0 {
		out.Country = make(map[string]string, len(g.Country))
		for k, v := range g.Country {
			out.Country[k] = v
		}
	}
	if len(g.Region) > 0 {
		out.Region = make(map[string]string, len(g.Region))
		for k, v := range g.Region {
			out.Region[k] = v
		}
	}
	return out
}
