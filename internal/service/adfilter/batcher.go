package adfilter

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Batcher coalesces messages from the same Guest within a sliding time window
// and judges them as a group. This lets the LLM catch advertising that is
// split across several innocent-looking messages — any single message in
// isolation may look fine, but the concatenated text reveals the pitch.
//
// Submit() returns immediately. After the window elapses the batcher invokes
// each item's OnApprove (NORMAL) or OnReject (AD) callback. Fail-open: any
// judge error flushes the batch as NORMAL.
type Batcher struct {
	judge          Judge
	window         time.Duration
	judgeTimeout   time.Duration
	logger         *zap.Logger

	mu      sync.Mutex
	batches map[BatchKey]*batchState
}

type BatchKey struct {
	BotID  uuid.UUID
	UserID int64
}

// BatchedItem represents one Telegram message held by the batcher.
type BatchedItem struct {
	// Text content of the message (empty for media without caption). Items
	// with empty Text are still buffered to preserve forwarding order, but
	// do not contribute to the text sent to the LLM.
	Text string
	// OnApprove is called when the batch is classified NORMAL (or the judge
	// fails and we fail-open). Must be safe to call from a goroutine.
	OnApprove func()
	// OnReject is called when the batch is classified AD. Called for every
	// item in the batch so each can be audit-logged independently. The first
	// item (IsFirst=true) is responsible for the single user-facing
	// notification — subsequent items should only audit.
	OnReject func(meta RejectMeta)
}

type RejectMeta struct {
	IsFirst   bool   // true on the first item only — use to send a single notification
	BatchSize int    // total items in the batch (use for plural wording)
	Reason    string // model-supplied short reason
}

type batchState struct {
	timer *time.Timer
	items []*BatchedItem
}

// NewBatcher builds a Batcher. window is the buffering delay applied to every
// incoming message before its batch is judged.
func NewBatcher(judge Judge, window time.Duration, logger *zap.Logger) *Batcher {
	if window <= 0 {
		window = 5 * time.Second
	}
	return &Batcher{
		judge:        judge,
		window:       window,
		judgeTimeout: 30 * time.Second,
		logger:       logger,
		batches:      make(map[BatchKey]*batchState),
	}
}

// Submit adds an item to the batch identified by key. If no batch exists for
// the key, one is created and a flush timer is started. Returns immediately.
func (b *Batcher) Submit(key BatchKey, item *BatchedItem) {
	if item == nil || item.OnApprove == nil || item.OnReject == nil {
		// Defensive — a caller missing a callback would deadlock the message.
		// Approve immediately so the message is not lost.
		if item != nil && item.OnApprove != nil {
			item.OnApprove()
		}
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	state, ok := b.batches[key]
	if !ok {
		state = &batchState{}
		b.batches[key] = state
		state.timer = time.AfterFunc(b.window, func() {
			b.flush(key)
		})
	}
	state.items = append(state.items, item)
}

// flush is invoked by the per-batch timer. Pops the batch from the map under
// lock, then runs the judge and dispatches outside the lock.
func (b *Batcher) flush(key BatchKey) {
	b.mu.Lock()
	state, ok := b.batches[key]
	if !ok {
		b.mu.Unlock()
		return
	}
	delete(b.batches, key)
	b.mu.Unlock()

	b.dispatch(key, state.items, true /* runJudge */)
}

// dispatch runs the judge (if runJudge) and invokes each item's callback.
// When runJudge is false (FlushAll on shutdown), all items are approved.
func (b *Batcher) dispatch(key BatchKey, items []*BatchedItem, runJudge bool) {
	if len(items) == 0 {
		return
	}

	if !runJudge {
		for _, it := range items {
			it.OnApprove()
		}
		return
	}

	combined := combineText(items)
	if combined == "" {
		// No text to judge — pass through.
		for _, it := range items {
			it.OnApprove()
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), b.judgeTimeout)
	defer cancel()

	result, err := b.judge.Judge(ctx, combined)
	if err != nil {
		b.logger.Warn("adfilter batcher: judge error, failing open",
			zap.String("bot_id", key.BotID.String()),
			zap.Int64("user_id", key.UserID),
			zap.Int("batch_size", len(items)),
			zap.Error(err))
		for _, it := range items {
			it.OnApprove()
		}
		return
	}

	if !result.IsAd {
		b.logger.Debug("adfilter batcher: classified NORMAL",
			zap.String("bot_id", key.BotID.String()),
			zap.Int64("user_id", key.UserID),
			zap.Int("batch_size", len(items)),
			zap.String("raw", result.Raw))
		for _, it := range items {
			it.OnApprove()
		}
		return
	}

	b.logger.Info("adfilter batcher: classified AD, dropping batch",
		zap.String("bot_id", key.BotID.String()),
		zap.Int64("user_id", key.UserID),
		zap.Int("batch_size", len(items)),
		zap.String("reason", result.Reason),
		zap.String("raw", result.Raw))

	meta := RejectMeta{BatchSize: len(items), Reason: result.Reason}
	for i, it := range items {
		m := meta
		m.IsFirst = i == 0
		it.OnReject(m)
	}
}

// FlushAll cancels all pending timers and forwards any buffered items
// without judging (fail-open). Call on shutdown so messages in flight are
// not lost.
func (b *Batcher) FlushAll() {
	b.mu.Lock()
	pending := make(map[BatchKey][]*BatchedItem, len(b.batches))
	for k, state := range b.batches {
		state.timer.Stop()
		pending[k] = state.items
	}
	b.batches = make(map[BatchKey]*batchState)
	b.mu.Unlock()

	if len(pending) == 0 {
		return
	}
	b.logger.Info("adfilter batcher: flushing pending batches on shutdown",
		zap.Int("batch_count", len(pending)))
	for k, items := range pending {
		b.dispatch(k, items, false /* runJudge */)
	}
}

func combineText(items []*BatchedItem) string {
	var sb strings.Builder
	first := true
	for _, it := range items {
		text := strings.TrimSpace(it.Text)
		if text == "" {
			continue
		}
		if !first {
			sb.WriteString("\n---\n")
		}
		sb.WriteString(text)
		first = false
	}
	return sb.String()
}
