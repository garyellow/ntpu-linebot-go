package logger

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultAsyncBufferSize   = 1024
	defaultAsyncFlushTimeout = 5 * time.Second
)

// AsyncOptions configures the async log pipeline.
type AsyncOptions struct {
	BufferSize   int
	FlushTimeout time.Duration
	OnDrop       func(total uint64)
}

type asyncRecord struct {
	ctx     context.Context
	record  slog.Record
	handler slog.Handler
}

type asyncWorker struct {
	ch           chan asyncRecord
	flushTimeout time.Duration
	closed       atomic.Bool
	wg           sync.WaitGroup
	ignored      atomic.Uint64
	onDrop       func(total uint64)
}

func newAsyncWorker(opts AsyncOptions) *asyncWorker {
	bufferSize := opts.BufferSize
	if bufferSize <= 0 {
		bufferSize = defaultAsyncBufferSize
	}
	flushTimeout := opts.FlushTimeout
	if flushTimeout <= 0 {
		flushTimeout = defaultAsyncFlushTimeout
	}

	w := &asyncWorker{
		ch:           make(chan asyncRecord, bufferSize),
		flushTimeout: flushTimeout,
		onDrop:       opts.OnDrop,
	}
	w.wg.Add(1)
	go w.run()
	return w
}

func (w *asyncWorker) run() {
	defer w.wg.Done()
	for rec := range w.ch {
		_ = rec.handler.Handle(rec.ctx, rec.record)
	}
}

func (w *asyncWorker) enqueue(ctx context.Context, record slog.Record, handler slog.Handler) {
	if w.closed.Load() {
		return
	}
	select {
	case w.ch <- asyncRecord{ctx: ctx, record: record, handler: handler}:
	default:
		newIgnored := w.ignored.Add(1)
		if w.onDrop != nil {
			w.onDrop(newIgnored)
		}
	}
}

func (w *asyncWorker) dropped() uint64 {
	return w.ignored.Load()
}

func (w *asyncWorker) shutdown(ctx context.Context) error {
	if w.closed.Swap(true) {
		return nil
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, w.flushTimeout)
		defer cancel()
	}
	close(w.ch)
	finished := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(finished)
	}()
	select {
	case <-finished:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// AsyncHandler wraps a slog.Handler and dispatches logs asynchronously.
// It is designed to prevent remote log shipping from blocking request paths.
type AsyncHandler struct {
	worker  *asyncWorker
	handler slog.Handler
	owner   bool
}

// NewAsyncHandler creates a new async handler with its own worker.
// Derived handlers created via WithAttrs/WithGroup share the same worker.
func NewAsyncHandler(handler slog.Handler, opts AsyncOptions) *AsyncHandler {
	return &AsyncHandler{
		worker:  newAsyncWorker(opts),
		handler: handler,
		owner:   true,
	}
}

// Enabled reports whether the underlying handler is enabled for the given level.
func (h *AsyncHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

// Handle enqueues the log record for async processing.
func (h *AsyncHandler) Handle(ctx context.Context, r slog.Record) error {
	if !h.handler.Enabled(ctx, r.Level) {
		return nil
	}
	h.worker.enqueue(ctx, r.Clone(), h.handler)
	return nil
}

// WithAttrs returns a new async handler with the attributes applied.
func (h *AsyncHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &AsyncHandler{
		worker:  h.worker,
		handler: h.handler.WithAttrs(attrs),
		owner:   false,
	}
}

// WithGroup returns a new async handler with the group applied.
func (h *AsyncHandler) WithGroup(name string) slog.Handler {
	return &AsyncHandler{
		worker:  h.worker,
		handler: h.handler.WithGroup(name),
		owner:   false,
	}
}

// Shutdown flushes pending logs up to the configured timeout.
func (h *AsyncHandler) Shutdown(ctx context.Context) error {
	if h == nil || h.worker == nil || !h.owner {
		return nil
	}
	return h.worker.shutdown(ctx)
}

// DroppedCount returns the total number of dropped log records.
func (h *AsyncHandler) DroppedCount() uint64 {
	if h == nil || h.worker == nil {
		return 0
	}
	return h.worker.dropped()
}
