package cmd

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/internal/controller"
)

func TestAdminListenEnabled(t *testing.T) {
	if !adminListenEnabled(true) {
		t.Fatal("default admin should listen")
	}
	if adminListenEnabled(false) {
		t.Fatal("disabled admin must not listen")
	}
}

func TestSkipAdminListenServesClientHealthOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	clientLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	engine := gin.New()
	controller.RegisterClient(engine, controller.Deps{
		Ready: func(context.Context) error { return nil },
	})
	srv := &http.Server{Handler: engine, ReadHeaderTimeout: time.Second}
	go func() { _ = srv.Serve(clientLn) }()
	t.Cleanup(func() { _ = srv.Close() })

	clientURL := "http://" + clientLn.Addr().String() + "/api/v1/health"
	resp, err := http.Get(clientURL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("client health=%d %s", resp.StatusCode, body)
	}

	var adminSrv *http.Server
	if adminListenEnabled(false) {
		adminSrv = &http.Server{Addr: "127.0.0.1:0"}
	}
	if adminSrv != nil {
		t.Fatal("enabled=false must not construct an admin http.Server")
	}
}
