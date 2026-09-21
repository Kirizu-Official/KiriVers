package controller

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// ClientProxyURLFromAddr 把本进程 client 监听地址转成管理平面反代目标。
// 空地址、仅通配绑定改写为 127.0.0.1，避免把请求打到非本机。
func ClientProxyURLFromAddr(addr string, useTLS bool) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		if strings.HasPrefix(addr, ":") {
			host, port = "127.0.0.1", strings.TrimPrefix(addr, ":")
		} else {
			return ""
		}
	}
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	scheme := "http"
	if useTLS {
		scheme = "https"
	}
	return scheme + "://" + net.JoinHostPort(host, port)
}

// mountClientProjectsProxy 在管理引擎上把 /api/v1/projects 与 /api/v1/projects/**
// 反代到本进程 client 平面。不匹配 /api/v1/admin。target 为空时不挂载，
// 单元测试保持 JSON 404。
func mountClientProjectsProxy(engine *gin.Engine, target string) {
	target = strings.TrimSpace(target)
	if target == "" {
		return
	}
	u, err := url.Parse(target)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(u)
	if u.Scheme == "https" {
		proxy.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // loopback same-process
		}
	}
	origDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		origDirector(req)
		req.Host = u.Host
		req.URL.Host = u.Host
		req.URL.Scheme = u.Scheme
		if prior, _, err := net.SplitHostPort(req.RemoteAddr); err == nil && prior != "" {
			req.Header.Set("X-Forwarded-For", prior)
		}
		if req.Header.Get("X-Forwarded-Host") == "" && req.Host != "" {
			req.Header.Set("X-Forwarded-Host", req.Header.Get("Host"))
		}
		if req.TLS != nil {
			req.Header.Set("X-Forwarded-Proto", "https")
		} else if req.Header.Get("X-Forwarded-Proto") == "" {
			req.Header.Set("X-Forwarded-Proto", "http")
		}
	}
	engine.Use(func(c *gin.Context) {
		p := c.Request.URL.Path
		if p != "/api/v1/projects" && !strings.HasPrefix(p, "/api/v1/projects/") {
			c.Next()
			return
		}
		proxy.ServeHTTP(&closeNotifyWriter{c.Writer}, c.Request)
		c.Abort()
	})
}

// closeNotifyWriter 让 httputil.ReverseProxy 在 httptest.ResponseRecorder /
// gin 包装 writer 上不 panic（缺少 CloseNotify）。
type closeNotifyWriter struct {
	gin.ResponseWriter
}

func (w closeNotifyWriter) CloseNotify() <-chan bool {
	return make(chan bool)
}
