package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
)

// OpenAsset validates ownership, expiry and integrity using a bounded hash pass.
func (s *Service) OpenAsset(ctx context.Context, user int64, id string) (Asset, *os.File, error) {
	a, err := s.Store.Asset(ctx, user, id)
	if err != nil {
		return a, nil, err
	}
	if a.Key != filepath.Base(a.Key) || (a.Key != a.ID+".image" && a.Key != a.ID+".attachment") {
		return a, nil, ErrNotFound
	}
	f, err := os.Open(filepath.Join(s.Root, a.Key))
	if err != nil {
		return a, nil, ErrNotFound
	}
	h := sha256.New()
	n, err := io.Copy(h, &assetContextReader{ctx: ctx, reader: f})
	if err == nil && (n != a.Size || hex.EncodeToString(h.Sum(nil)) != a.SHA) {
		err = Invalid("AGENT_ASSET_INTEGRITY_FAILED")
	}
	if err == nil {
		_, err = f.Seek(0, io.SeekStart)
	}
	if err != nil {
		f.Close()
		return a, nil, err
	}
	return a, f, nil
}

type assetContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *assetContextReader) Read(b []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(b)
}
