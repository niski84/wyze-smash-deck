package wyzeferal

import (
	"context"
	"log"
	"sync"
	"time"
)

const (
	streamRefreshInterval = 3 * time.Minute  // normal cadence — well within TURN TTL (~5 min)
	streamStaleCutoff     = 4 * time.Minute  // serve stale if refresh fails, up to this age
	streamRetryBase       = 30 * time.Second // first retry delay after failure
	streamRetryMax        = 2 * time.Minute  // cap on backoff
)

// streamRefresher keeps fresh WebRTC stream params in memory.
// A background goroutine polls get-streams every 3 minutes so the frontend
// never has to wait for a cold call. On failure it backs off and retries,
// and recovers from any panic so it never silently dies.
type streamRefresher struct {
	mu     sync.RWMutex
	latest *StreamParams

	client      *WyzeStreamClient
	deviceID    string
	deviceModel string
}

func newStreamRefresher(client *WyzeStreamClient, deviceID, deviceModel string) *streamRefresher {
	return &streamRefresher{
		client:      client,
		deviceID:    deviceID,
		deviceModel: deviceModel,
	}
}

func (r *streamRefresher) get() *StreamParams {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.latest
}

func (r *streamRefresher) set(p *StreamParams) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.latest = p
}

// refresh calls get-streams and updates in-memory state.
func (r *streamRefresher) refresh(ctx context.Context) error {
	p, err := r.client.GetStreamParams(ctx, r.deviceID, r.deviceModel)
	if err != nil {
		return err
	}
	r.set(p)
	log.Printf("[stream-refresh] captured %s age=0s", p.DeviceID)
	return nil
}

// start runs the background refresh loop until ctx is cancelled.
// On failures it backs off exponentially up to streamRetryMax, then resumes
// the normal 3-minute cadence once a refresh succeeds again.
// A deferred recover ensures the goroutine never silently dies from a panic.
func (r *streamRefresher) start(ctx context.Context) {
	defer func() {
		if v := recover(); v != nil {
			log.Printf("[stream-refresh] PANIC recovered: %v — restarting in 30s", v)
			time.Sleep(30 * time.Second)
			go r.start(ctx) // restart the goroutine
		}
	}()

	// Initial fetch — retry with backoff until it succeeds or ctx is done.
	for {
		if err := r.refresh(ctx); err != nil {
			log.Printf("[stream-refresh] initial fetch failed: %v — retrying in 30s", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(streamRetryBase):
				continue
			}
		}
		break
	}

	retryDelay := streamRetryBase
	consecutiveFails := 0

	ticker := time.NewTicker(streamRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.refresh(ctx); err != nil {
				consecutiveFails++
				log.Printf("[stream-refresh] refresh failed (attempt %d): %v — retrying in %s", consecutiveFails, err, retryDelay)

				ticker.Reset(retryDelay)
				retryDelay = min(retryDelay*2, streamRetryMax)
			} else {
				if consecutiveFails > 0 {
					log.Printf("[stream-refresh] recovered after %d failed attempt(s)", consecutiveFails)
				}
				consecutiveFails = 0
				retryDelay = streamRetryBase
				ticker.Reset(streamRefreshInterval)
			}
		}
	}
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
