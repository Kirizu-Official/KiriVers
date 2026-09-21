// Package update 是更新目标选择（check 选目标）的唯一实现。
//
// 职责边界（task design §1）：
//   - Catalog 快照类型与 CatalogLoader 接口（GORM 实现与内存实现位于 internal/repository）；
//   - SelectTarget 纯函数：跨渠道升级、强制/灰度逐候选判定、中继截断（≤8 跳）、
//     吊销/yank 降级分支、fallback_arch 回退；
//   - changelog 查询引擎（scope/layout/locale 回退）；
//   - check 编排：能力集合、ETag/Cache-Control/Vary、签名注入。
//
// 所有后继 feed 子任务（Sparkle/electron/Tauri 等）必须复用 SelectTarget，
// 传入「匿名、无 device_id、默认 hw_rev」的请求，不得复刻算法。
// 本包不依赖 internal/service 与 internal/repository（避免 repository → update →
// service → repository 导入环）；版本引用解析在 compare.go 内实现，
// 并由外部测试包与 service.ParseVersionRef 做逐例对照。
package update
