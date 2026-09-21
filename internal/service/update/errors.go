package update

import "errors"

// integrity / diff 端点新增的领域错误（docs/app-init.md §8 / §10.3 / §12.2）。
// HTTP 状态码映射集中在 client controller 的统一错误出口。
var (
	// ErrVersionNotVisible 目标 Version 为 Draft，或平台线尚未 packs_ready_at
	// → HTTP 404 VERSION_NOT_VISIBLE。
	ErrVersionNotVisible = errors.New("version is not visible to clients")
	// ErrVersionRevoked Version 已吊销 → HTTP 409 VERSION_REVOKED，
	// 错误响应内禁止出现任何下载 URL（C09-2）。
	ErrVersionRevoked = errors.New("version has been revoked")
	// ErrVersionLineNotFound 该 Version 无此 (os,arch) 线、线未就绪或被 yank，
	// 或无兼容 hw 变体 → HTTP 404 VERSION_LINE_NOT_FOUND（§12.2）。
	ErrVersionLineNotFound = errors.New("version line not found for the requested platform")
	// ErrChannelConflict 可选 channel 参数与该 Version 的渠道不一致
	// → HTTP 400 CHANNEL_CONFLICT（C09-10：不得假装另一渠道的版本）。
	ErrChannelConflict = errors.New("channel does not match the version channel")
	// ErrPreconditionFailed 指定 target 非强制且灰度未命中
	// → HTTP 412 PRECONDITION_FAILED，不下发任何包（C09-11）。
	ErrPreconditionFailed = errors.New("gray rollout miss for the specified target")
	// ErrLineDetailsUnavailable Service 未注入 LineDetailSource 却被 integrity/diff
	// 调用 → 属装配错误，HTTP 层按 500 处理（check 路径不受影响）。
	ErrLineDetailsUnavailable = errors.New("line detail source is not configured")
	// ErrNotFound 渠道 Token 不匹配等不泄露原因的缺失 → HTTP 404 NOT_FOUND。
	ErrNotFound = errors.New("not found")
)
