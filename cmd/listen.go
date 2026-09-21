package cmd

import (
	"net/http"
	"strings"
)

// startServer 在证书与私钥都配置时启用 TLS，否则明文 HTTP。
func startServer(srv *http.Server, certFile, keyFile string) error {
	if listenMode(certFile, keyFile) == "tls" {
		return srv.ListenAndServeTLS(certFile, keyFile)
	}
	return srv.ListenAndServe()
}

// listenMode 把 TLS 判定抽出来，便于单测且不需要真实证书。
func listenMode(certFile, keyFile string) string {
	if strings.TrimSpace(certFile) != "" && strings.TrimSpace(keyFile) != "" {
		return "tls"
	}
	return "plain"
}
