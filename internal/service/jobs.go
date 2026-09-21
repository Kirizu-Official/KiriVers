package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
)

const jobIdleSleep = time.Second

// JobHandler 处理已抢到的任务。本任务不注册任何类型。
type JobHandler func(ctx context.Context, jobType string, payload []byte) error

// JobWorker 启动 N 个 goroutine，循环 SELECT ... FOR UPDATE SKIP LOCKED。
type JobWorker struct {
	store    repository.JobStore
	handlers map[string]JobHandler
	n        int
	log      zerolog.Logger
	nodeID   uuid.UUID
	mu       sync.RWMutex
}

// NewJobWorker 构造 worker 池。n <= 0 时 Start 为空操作。log 使用进程系统 logger 的 mod=job 子 logger。
func NewJobWorker(store repository.JobStore, n int, log zerolog.Logger) *JobWorker {
	return &JobWorker{
		store:    store,
		handlers: map[string]JobHandler{},
		n:        n,
		log:      log,
	}
}

// StartJobWorkers 启动 jobs.workers 个协程，返回停止函数（取消并等待退出）。
func StartJobWorkers(ctx context.Context, store repository.JobStore, n int, log zerolog.Logger) func() {
	return NewJobWorker(store, n, log).Start(ctx)
}

// Start 在独立 goroutine 中抢任务。无 handler 时把行释放回 queued 并休眠，避免热循环。
func (w *JobWorker) Start(ctx context.Context) func() {
	if w == nil || w.store == nil || w.n <= 0 {
		return func() {}
	}
	ctx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	for i := 0; i < w.n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.loop(ctx)
		}()
	}
	return func() {
		cancel()
		wg.Wait()
	}
}

// SetNodeID 设置本进程节点 UUID，Claim 只抢 owner 为空或等于该 ID 的行。
func (w *JobWorker) SetNodeID(id uuid.UUID) {
	if w == nil {
		return
	}
	w.mu.Lock()
	w.nodeID = id
	w.mu.Unlock()
}

// RegisterHandler 为指定的任务类型注册处理函数。
func (w *JobWorker) RegisterHandler(jobType string, handler JobHandler) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.handlers == nil {
		w.handlers = make(map[string]JobHandler)
	}
	w.handlers[jobType] = handler
}

func (w *JobWorker) claimFilter() repository.ClaimFilter {
	w.mu.RLock()
	defer w.mu.RUnlock()
	types := make([]string, 0, len(w.handlers))
	for typ := range w.handlers {
		types = append(types, typ)
	}
	return repository.ClaimFilter{NodeID: w.nodeID, Types: types}
}

func (w *JobWorker) handler(jobType string) (JobHandler, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	h, ok := w.handlers[jobType]
	return h, ok
}

func (w *JobWorker) loop(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		job, err := w.store.Claim(ctx, w.claimFilter())
		if err != nil {
			if shouldLogClaimErr(err) {
				w.log.Error().Err(err).Msg("job claim failed")
			}
			if !sleepOrDone(ctx, jobIdleSleep) {
				return
			}
			continue
		}
		if job == nil {
			if !sleepOrDone(ctx, jobIdleSleep) {
				return
			}
			continue
		}
		handler, ok := w.handler(job.Type)
		if !ok {
			_ = w.store.Release(ctx, job.ID)
			if !sleepOrDone(ctx, jobIdleSleep) {
				return
			}
			continue
		}
		if err := handler(ctx, job.Type, job.Payload); err != nil {
			w.log.Error().Err(err).Str("job_type", job.Type).Msg("job handler failed")
			_ = w.store.MarkFailed(ctx, job.ID, err.Error())
			continue
		}
		if current, err := w.store.GetByID(ctx, job.ID); err == nil && current != nil {
			if current.Status == model.JobStatusRunning {
				_ = w.store.MarkSucceeded(ctx, job.ID)
			}
		} else {
			_ = w.store.MarkSucceeded(ctx, job.ID)
		}
	}
}

func sleepOrDone(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func shouldLogClaimErr(err error) bool {
	return err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}
