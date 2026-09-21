package store

import (
	"sort"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
)

// VisibleItem 是匿名可见版本投影集的一项：Version、其 (os, arch) 切片与
// 该切片上匹配 hw_rev 的 kind=full 产物。
//
// 三元组由 update.SelectTarget 同源口径选出（列表型 feed 不跑完整 SelectTarget，
// 而是对目录做 §4.4 候选过滤，见 AnonymousVisible），保证商店 feed 与原生
// check 看到的「版本 + 线 + 包」永不漂移。
type VisibleItem struct {
	Version *update.VersionState
	Line    *update.LineState
	Package *update.ArtifactInfo
	// Rank 是所属渠道的 stability_rank（§4.4；比较键相同时排序用）。
	Rank int
}

// AnonymousVisible 返回目录快照中对「匿名客户端（无 device_id、默认 hw_rev、
// 指定渠道）」可见的版本集，按项目 compare_engine 的比较键新→旧排序
// （同键取渠道 stability_rank 更高者，§4.4 步骤 4）。
//
// 可见性口径（§4.4 末条 / §9 / C16-5，与 update 包同一套目录语义）：
//   - Status=published（draft/deprecated/revoked 一律不可见）；
//   - 渠道命中且启用；
//   - 本 (os, arch) 切片 ready 且 PacksReadyAt 已盖戳（系统预热完成）；
//   - 默认 hw 变体（HwRev=nil）的 kind=full 产物存在——匿名客户端不声明
//     hw_rev，只匹配默认变体（C08-10 / C16-8：未声明 hw 的客户端不得只
//     收到非默认变体）；
//   - 灰度命中 = 有效百分比（per-line 覆盖优先）==100，或强制可见
//     （IsCritical：关键版本对全员强制，忽略灰度，§5.4）。匿名无 device_id，
//     非强制且灰度 <100% 一律不可见（白名单对匿名同样不生效，C12-5/6）。
//
// engine 是项目 compare_engine，用于排序键提取；缺键版本跳过（发布闸门
// 已保证 Published 版本齐备，此处兜底）。
func AnonymousVisible(cat *update.Catalog, engine, channel, os, arch string) []VisibleItem {
	if cat == nil {
		return nil
	}
	// 渠道解析一次（§4.3）：渠道不存在或已禁用时无可投影版本（Adapter 层
	// 已对未知渠道报 400，此处兜底返回空集）。
	ch, channelOK := cat.Channel(channel)
	if !channelOK || !ch.Enabled {
		return nil
	}
	items := make([]VisibleItem, 0, len(cat.Versions))
	for i := range cat.Versions {
		vs := &cat.Versions[i]
		v := &vs.Version
		if v.Status != model.VersionStatusPublished {
			continue
		}
		if v.ChannelSlug != channel {
			continue
		}
		line := vs.Line(os, arch)
		if !update.LinePacksReady(line) {
			continue
		}
		pkg := defaultVariant(line)
		if pkg == nil {
			continue
		}
		if !v.GrayIsComplete() {
			continue
		}
		items = append(items, VisibleItem{Version: vs, Line: line, Package: pkg, Rank: ch.StabilityRank})
	}
	sortVisibleDesc(cat.Project.CompareEngine, items)
	return items
}

// sortVisibleDesc 按比较键降序排序（同键渠道 rank 高者在前）。
func AnonymousVisibleAllPublic(cat *update.Catalog, engine, os, arch string) []VisibleItem {
	if cat == nil {
		return nil
	}
	items := make([]VisibleItem, 0, len(cat.Versions))
	for i := range cat.Versions {
		vs := &cat.Versions[i]
		v := &vs.Version
		if v.Status != model.VersionStatusPublished {
			continue
		}
		ch, channelOK := cat.Channel(v.ChannelSlug)
		if !channelOK || !ch.Enabled || ch.Unlisted {
			continue
		}
		line := vs.Line(os, arch)
		if !update.LinePacksReady(line) {
			continue
		}
		pkg := defaultVariant(line)
		if pkg == nil {
			continue
		}
		if !v.GrayIsComplete() {
			continue
		}
		items = append(items, VisibleItem{Version: vs, Line: line, Package: pkg, Rank: ch.StabilityRank})
	}
	sortVisibleDesc(engine, items)
	return items
}

func sortVisibleDesc(engine string, items []VisibleItem) {
	sort.SliceStable(items, func(i, j int) bool {
		c, ok := update.CompareVersions(engine, &items[i].Version.Version, &items[j].Version.Version)
		if ok && c != 0 {
			return c > 0
		}
		return items[i].Rank > items[j].Rank
	})
}

// defaultVariant 返回线上的默认 hw 变体（HwRev=nil）kind=full 产物；
// 不存在返回 nil（匿名口径下该线对本平台不可见）。
func defaultVariant(line *update.LineState) *update.ArtifactInfo {
	if line == nil {
		return nil
	}
	for i := range line.FullPkgs {
		p := &line.FullPkgs[i]
		if p.HwRev == nil || *p.HwRev == "" {
			return p
		}
	}
	return nil
}
