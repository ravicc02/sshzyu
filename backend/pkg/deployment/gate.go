package deployment

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"
)

type Gate struct {
	path        string
	mu          sync.Mutex
	inflight    int64
	ctx         context.Context
	cancel      context.CancelFunc
	stateMu     sync.Mutex
	cachedState State
	cachedErr   error
	cacheUntil  time.Time
	cachedInfo  os.FileInfo
}

type State struct {
	Active      bool   `json:"active"`
	OperationID string `json:"operation_id"`
}

func NewGate(path string) *Gate {
	ctx, cancel := context.WithCancel(context.Background())
	return &Gate{path: path, ctx: ctx, cancel: cancel}
}

var Default = NewGate(os.Getenv("SSHZY_UPDATE_MAINTENANCE_FILE"))

func (gate *Gate) state() (State, error) {
	if gate.path == "" {
		return State{}, nil
	}
	gate.stateMu.Lock()
	defer gate.stateMu.Unlock()
	now := time.Now()
	info, statErr := os.Stat(gate.path)
	if statErr != nil {
		gate.cacheUntil = time.Time{}
		return State{Active: true}, errors.New("maintenance state unavailable")
	}
	if now.Before(gate.cacheUntil) && gate.cachedInfo != nil && os.SameFile(info, gate.cachedInfo) && info.ModTime() == gate.cachedInfo.ModTime() && info.Size() == gate.cachedInfo.Size() && info.Mode() == gate.cachedInfo.Mode() {
		return gate.cachedState, gate.cachedErr
	}
	state, err := gate.readState()
	gate.cachedState, gate.cachedErr = state, err
	gate.cachedInfo = info
	gate.cacheUntil = now.Add(500 * time.Millisecond)
	return state, err
}

func (gate *Gate) readState() (State, error) {
	data, err := os.ReadFile(gate.path)
	if err != nil || len(data) > 8192 {
		return State{Active: true}, errors.New("maintenance state unavailable")
	}
	var state State
	if json.Unmarshal(data, &state) != nil {
		return State{Active: true}, errors.New("maintenance state invalid")
	}
	return state, nil
}

func (gate *Gate) Status() (State, int64, error) {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	state, err := gate.state()
	return state, gate.inflight, err
}

func (gate *Gate) Inflight() int64 {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	return gate.inflight
}

func (gate *Gate) Enter() (func(), error) {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	state, err := gate.state()
	if err != nil || state.Active {
		return nil, errors.New("deployment maintenance active")
	}
	gate.inflight++
	var once sync.Once
	return func() {
		once.Do(func() {
			gate.mu.Lock()
			gate.inflight--
			gate.mu.Unlock()
		})
	}, nil
}

func (gate *Gate) Wait(ctx context.Context) error {
	for {
		state, _, err := gate.Status()
		if err == nil && !state.Active {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func StartWhenOpen(start func()) {
	state, _, err := Default.Status()
	if err == nil && !state.Active {
		start()
		return
	}
	go func() {
		if Default.Wait(Default.ctx) == nil {
			done, err := Default.Enter()
			if err == nil {
				defer done()
				start()
			}
		}
	}()
}

func (gate *Gate) Close() { gate.cancel() }
