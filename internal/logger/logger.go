// Package logger 初始化 Zerolog：控制台 ConsoleWriter、文件 JSON + lumberjack 滚动。
package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/Kirizu-Official/KiriVers/internal/config"
)

// 结构化字段名与稳定取值（过滤/检索约定）。
const (
	FieldMod   = "mod"
	FieldCat   = "cat"
	FieldPlane = "plane"

	CatSystem = "system"
	CatAccess = "access"

	PlaneAdmin  = "admin"
	PlaneClient = "client"

	ModCmd     = "cmd"
	ModDB      = "db"
	ModStorage = "storage"
	ModCache   = "cache"
	ModJob     = "job"
	ModAudit   = "audit"
	ModGin     = "gin"
	ModHTTP    = "http"
)

type nopCloser struct{}

func (nopCloser) Close() error { return nil }

// Open 按 StreamConfig 打开一条日志流。ctx 须已含 cat（平面流再含 plane）。
// 非法级别回退 info；双 sink 全关或启用文件但路径为空则返回 error（不会落到 TempDir）。
func Open(cfg config.StreamConfig, ctx zerolog.Context) (zerolog.Logger, io.Closer, error) {
	zerolog.TimeFieldFormat = time.RFC3339
	w, closer, err := buildWriter(cfg)
	if err != nil {
		return zerolog.Nop(), nil, err
	}
	log := ctx.Timestamp().Logger().Output(w).Level(parseLevel(cfg.Level))
	return log, closer, nil
}

func buildWriter(cfg config.StreamConfig) (io.Writer, io.Closer, error) {
	if !cfg.Console.Enabled && !cfg.File.Enabled {
		return nil, nil, fmt.Errorf("log stream: console and file sinks are both disabled")
	}

	var writers []io.Writer
	var closer io.Closer = nopCloser{}

	if cfg.Console.Enabled {
		writers = append(writers, zerolog.ConsoleWriter{
			Out:        os.Stderr,
			TimeFormat: time.RFC3339,
			NoColor:    cfg.Console.NoColor,
		})
	}

	if cfg.File.Enabled {
		dir := strings.TrimSpace(cfg.File.Dir)
		name := strings.TrimSpace(cfg.File.Filename)
		if dir == "" || name == "" {
			return nil, nil, fmt.Errorf("log stream: file enabled but dir or filename is empty")
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, nil, fmt.Errorf("log stream: mkdir %s: %w", dir, err)
		}
		lj := &lumberjack.Logger{
			Filename:   filepath.Join(dir, name),
			MaxSize:    cfg.File.MaxSizeMB,
			MaxBackups: cfg.File.MaxBackups,
			MaxAge:     cfg.File.MaxAgeDays,
			Compress:   cfg.File.Compress,
			LocalTime:  cfg.File.LocalTime,
		}
		writers = append(writers, lj)
		closer = lj
	}

	if len(writers) == 1 {
		return writers[0], closer, nil
	}
	return zerolog.MultiLevelWriter(writers...), closer, nil
}

func parseLevel(level string) zerolog.Level {
	level = strings.ToLower(strings.TrimSpace(level))
	// 空字符串在 Zerolog 中是 NoLevel（全放行），按非法级别回退 info。
	if level == "" {
		return zerolog.InfoLevel
	}
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		return zerolog.InfoLevel
	}
	return lvl
}

// NewWithWriter 便于测试注入输出（JSON，不含 ConsoleWriter）。
func NewWithWriter(w io.Writer, level zerolog.Level) zerolog.Logger {
	zerolog.TimeFieldFormat = time.RFC3339
	return zerolog.New(w).Level(level).With().Timestamp().Logger()
}

// NewStdWriter 把 gin.DefaultWriter 写入转为 Zerolog 行：含 `[GIN-debug]` 的为 Debug，其余为 Info。
func NewStdWriter(log zerolog.Logger) io.Writer {
	return stdWriter{log: log}
}

// NewErrorWriter 把 gin.DefaultErrorWriter 写入转为 Zerolog Error 行。
func NewErrorWriter(log zerolog.Logger) io.Writer {
	return errorWriter{log: log}
}

type stdWriter struct {
	log zerolog.Logger
}

func (w stdWriter) Write(p []byte) (int, error) {
	msg := strings.TrimSpace(string(p))
	if msg == "" {
		return len(p), nil
	}
	if strings.Contains(msg, "[GIN-debug]") {
		w.log.Debug().Msg(msg)
		return len(p), nil
	}
	w.log.Info().Msg(msg)
	return len(p), nil
}

type errorWriter struct {
	log zerolog.Logger
}

func (w errorWriter) Write(p []byte) (int, error) {
	msg := strings.TrimSpace(string(p))
	if msg == "" {
		return len(p), nil
	}
	w.log.Error().Msg(msg)
	return len(p), nil
}
