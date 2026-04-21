package wyzeferal

import (
	"context"
	"log"
	"sync"
	"time"
)

const (
	streamRefreshInterval = 3 * time.Minute  // refresh well within TURN TTL (~5 min)
	streamStaleCutoff     = 4 * time.Minute  // serve stale if refresh fails, up to this age
)

// streamRefresher keeps fresh WebRTC stream params in memory.
// A background goroutine polls get-streams every 3 minutes so the frontend
// never has to wait for a cold call.
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
// It performs an initial fetch immediately, then on every interval tick.
func (r *streamRefresher) start(ctx context.Context) {
	if err := r.refresh(ctx); err != nil {
		log.Printf("[stream-refresh] initial fetch failed: %v", err)
	}

	ticker := time.NewTicker(streamRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.refresh(ctx); err != nil {
				log.Printf("[stream-refresh] refresh failed: %v", err)
			}
		}
	}
}
