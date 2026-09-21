// Package service 存放核心业务逻辑：更新计算、差分计划、哈希校验等。
// 本包不得依赖 gin.Context，也不得绕过 pkg/response 手写 HTTP 错误。
package service
