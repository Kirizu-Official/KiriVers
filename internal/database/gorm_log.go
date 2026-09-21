package database

import (
	"context"
	"errors"
	"time"

	"github.com/rs/zerolog"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// gormZerolog 把 GORM Warn/Error/慢查询写入 Zerolog。有效级别保持 Warn（200ms），
// 不把每条 SQL 打到进程 debug；着色交给 ConsoleWriter。
type gormZerolog struct {
	log            zerolog.Logger
	slowThreshold  time.Duration
	ignoreNotFound bool
}

func newGormLogger(log zerolog.Logger) gormlogger.Interface {
	return &gormZerolog{
		log:            log,
		slowThreshold:  200 * time.Millisecond,
		ignoreNotFound: true,
	}
}

func (g *gormZerolog) LogMode(gormlogger.LogLevel) gormlogger.Interface {
	return g
}

func (g *gormZerolog) Info(context.Context, string, ...interface{}) {
	// 有效级别 Warn：忽略 GORM Info（含常规 SQL 调试）。
}

func (g *gormZerolog) Warn(_ context.Context, msg string, data ...interface{}) {
	g.log.Warn().Msgf(msg, data...)
}

func (g *gormZerolog) Error(_ context.Context, msg string, data ...interface{}) {
	g.log.Error().Msgf(msg, data...)
}

func (g *gormZerolog) Trace(_ context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	elapsed := time.Since(begin)
	switch {
	case err != nil && !(g.ignoreNotFound && errors.Is(err, gorm.ErrRecordNotFound)):
		sql, rows := fc()
		g.log.Error().Err(err).Dur("elapsed", elapsed).Int64("rows", rows).Str("sql", sql).Msg("gorm query")
	case g.slowThreshold > 0 && elapsed > g.slowThreshold:
		sql, rows := fc()
		g.log.Warn().Dur("elapsed", elapsed).Int64("rows", rows).Str("sql", sql).Msg("slow sql")
	}
}
