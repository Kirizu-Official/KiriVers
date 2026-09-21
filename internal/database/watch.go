package database

import (
	"context"
	"database/sql/driver"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"gorm.io/gorm"
)

const (
	// DefaultReconnectInterval 是运行中 Postgres Ping 探测 / 重连周期。
	DefaultReconnectInterval = 5 * time.Second
	defaultPingTimeout       = 3 * time.Second
)

// Watcher 在成功 Open 之后探测连接：失败时 Available=false（HTTP 层对 /api 业务路径
// 回 404），并按间隔 Ping 直到恢复。不在运行中退出进程。
type Watcher struct {
	ping     func(context.Context) error
	interval time.Duration
	timeout  time.Duration
	log      zerolog.Logger

	ok atomic.Bool
}

// Watch 从已打开的 db 开始探测。ctx 取消后停止循环。interval<=0 时用 DefaultReconnectInterval。
func Watch(ctx context.Context, db *gorm.DB, interval time.Duration, log zerolog.Logger) *Watcher {
	if interval <= 0 {
		interval = DefaultReconnectInterval
	}
	w := &Watcher{
		ping: func(ctx context.Context) error {
			return Ping(ctx, db)
		},
		interval: interval,
		timeout:  defaultPingTimeout,
		log:      log,
	}
	w.ok.Store(true)
	if db != nil {
		w.attach(db)
	}
	go w.loop(ctx)
	return w
}

// Available 为 false 时业务 API 应按 NoRoute 语义返回 404 NOT_FOUND。
func (w *Watcher) Available() bool {
	if w == nil {
		return true
	}
	return w.ok.Load()
}

func (w *Watcher) loop(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.probe(ctx)
		}
	}
}

func (w *Watcher) probe(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, w.timeout)
	err := w.ping(ctx)
	cancel()
	if parent.Err() != nil {
		return
	}
	if err != nil {
		w.markDown(err)
		return
	}
	w.markUp()
}

func (w *Watcher) markDown(err error) {
	if w.ok.CompareAndSwap(true, false) {
		w.log.Error().Err(err).Msg("database unavailable; api returns 404 until reconnect")
	}
}

func (w *Watcher) markUp() {
	if w.ok.CompareAndSwap(false, true) {
		w.log.Info().Msg("database reconnected")
	}
}

func (w *Watcher) attach(db *gorm.DB) {
	hook := func(tx *gorm.DB) {
		if tx == nil {
			return
		}
		if IsUnavailable(tx.Error) {
			w.markDown(tx.Error)
		}
	}
	_ = db.Callback().Query().After("gorm:after_query").Register("kirivers:watch_query", hook)
	_ = db.Callback().Create().After("gorm:after_create").Register("kirivers:watch_create", hook)
	_ = db.Callback().Update().After("gorm:after_update").Register("kirivers:watch_update", hook)
	_ = db.Callback().Delete().After("gorm:after_delete").Register("kirivers:watch_delete", hook)
	_ = db.Callback().Row().After("gorm:row").Register("kirivers:watch_row", hook)
	_ = db.Callback().Raw().After("gorm:raw").Register("kirivers:watch_raw", hook)
}

// IsUnavailable 判断错误是否表示连接不可用（而非 RecordNotFound / 请求取消）。
func IsUnavailable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false
	}
	if errors.Is(err, driver.ErrBadConn) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && !netErr.Timeout() {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, n := range unavailableNeedles {
		if strings.Contains(msg, n) {
			return true
		}
	}
	return false
}

var unavailableNeedles = [...]string{
	"connection refused",
	"connectex",
	"no such host",
	"network is unreachable",
	"connection reset",
	"broken pipe",
	"conn closed",
	"connection closed",
	"sql: database is closed",
	"bad connection",
	"failed to connect",
	"dial tcp",
	"the database system is starting up",
	"the database system is shutting down",
	"too many clients already",
	"unexpected eof",
}
