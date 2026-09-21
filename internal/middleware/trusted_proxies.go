package middleware

import "github.com/gin-gonic/gin"

// ApplyTrustedProxies 为平面引擎设置信任的反向代理。
// 空列表覆盖 Gin 默认的信任全网：SetTrustedProxies(nil) 后 ClientIP 为 RemoteAddr 主机部分。
func ApplyTrustedProxies(engine *gin.Engine, proxies []string) error {
	if len(proxies) == 0 {
		return engine.SetTrustedProxies(nil)
	}
	return engine.SetTrustedProxies(proxies)
}
