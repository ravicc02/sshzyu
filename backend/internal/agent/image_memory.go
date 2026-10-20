package agent

import (
	"bytes"
	"context"
	"image"
	"time"
)

// Bound only large image materialization, not upstream request concurrency.
var imageMemoryGate = make(chan struct{}, 1)

func AcquireImageMemory(ctx context.Context) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case imageMemoryGate <- struct{}{}:
		return func() { <-imageMemoryGate }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type previewEntry struct {
	data    []byte
	expires time.Time
	last    time.Time
}

const maxPreviewCacheBytes = 16 << 20

// Thumbnail bytes are bounded and private; originals remain unchanged on disk.
func (s *Service) Thumbnail(ctx context.Context, user int64, id string) ([]byte, error) {
	a, err := s.Store.Asset(ctx, user, id)
	if err != nil {
		return nil, err
	}
	if len(a.MIME) < 6 || a.MIME[:6] != "image/" {
		return nil, Invalid("AGENT_INVALID_FILE")
	}
	key := a.ID + ":" + a.SHA
	cached := func() []byte {
		s.previewMu.Lock()
		defer s.previewMu.Unlock()
		entry, ok := s.previews[key]
		if ok && time.Now().Before(entry.expires) {
			entry.last = time.Now()
			s.previews[key] = entry
			return entry.data
		}
		return nil
	}
	if b := cached(); b != nil {
		return b, nil
	}
	release, err := AcquireImageMemory(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	if b := cached(); b != nil {
		return b, nil
	}
	a, err = s.LoadAsset(ctx, user, id)
	if err != nil {
		return nil, err
	}
	b, err := scaledJPEG(a.Data, 40000000, 896, 75)
	if err != nil {
		return nil, err
	}
	if len(b) > 1<<20 {
		return nil, Invalid("AGENT_THUMBNAIL_LIMIT")
	}
	s.previewMu.Lock()
	defer s.previewMu.Unlock()
	if s.previews == nil {
		s.previews = make(map[string]previewEntry)
	}
	total := len(b)
	for _, entry := range s.previews {
		total += len(entry.data)
	}
	for len(s.previews) >= 128 || total > maxPreviewCacheBytes {
		oldest := ""
		var at time.Time
		for key, entry := range s.previews {
			if oldest == "" || entry.last.Before(at) {
				oldest = key
				at = entry.last
			}
		}
		if oldest == "" {
			break
		}
		total -= len(s.previews[oldest].data)
		delete(s.previews, oldest)
	}
	s.previews[key] = previewEntry{data: b, expires: a.Expires, last: time.Now()}
	return b, nil
}
func (s *Service) expirePreviews() {
	s.previewMu.Lock()
	defer s.previewMu.Unlock()
	for key, entry := range s.previews {
		if !time.Now().Before(entry.expires) {
			delete(s.previews, key)
		}
	}
}

// Encoding helper used only while holding the process image-memory lease.
func scaledJPEG(data []byte, pixels int64, side, quality int) ([]byte, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > pixels {
		return nil, Invalid("AGENT_VISION_IMAGE_TOO_LARGE")
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, Invalid("AGENT_INVALID_FILE")
	}
	return encodeScaledJPEG(src, cfg.Width, cfg.Height, side, quality)
}
