package scheduler

import (
	"context"
	"log/slog"
	"time"

	"team-invite/internal/database"
)

type QuotaRunner struct {
	store    *database.Store
	interval time.Duration
	logger   *slog.Logger
	stopCh   chan struct{}
}

func NewQuotaRunner(store *database.Store, interval time.Duration, logger *slog.Logger) *QuotaRunner {
	return &QuotaRunner{
		store:    store,
		interval: interval,
		logger:   logger,
		stopCh:   make(chan struct{}),
	}
}

func (r *QuotaRunner) Start(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				r.check(ctx)
			case <-ctx.Done():
				return
			case <-r.stopCh:
				return
			}
		}
	}()
}

func (r *QuotaRunner) Stop() {
	close(r.stopCh)
}

func (r *QuotaRunner) check(ctx context.Context) {
	schedule, err := r.store.LoadQuotaSchedule(ctx)
	if err != nil || schedule == nil {
		return
	}
	if time.Now().Before(schedule.ApplyAt) {
		return
	}
	if err := r.store.UpdateQuota(ctx, schedule.Target); err != nil {
		r.logger.Error("failed to apply quota schedule", "error", err)
		return
	}
	if err := r.store.ClearQuotaSchedule(ctx); err != nil {
		r.logger.Warn("failed to clear quota schedule", "error", err)
	}
	r.logger.Info("quota schedule applied", "target", schedule.Target, "author", schedule.Author)
}
