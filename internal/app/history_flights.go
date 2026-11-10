package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"ta/internal/concurrency"
)

type historyFlight struct {
	commandID string
	hash      string
	cancel    context.CancelFunc
	done      chan struct{}
	value     any
	err       error
}

func historyRequestHash(kind string, request any) string {
	data, _ := json.Marshal(struct {
		Kind    string `json:"kind"`
		Request any    `json:"request"`
	}{kind, request})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// historyRun serializes one campaign's mutating/AI work without holding a disk
// lease during a model call. Matching concurrent retries await the same result.
// Persistent command receipts handle retries after restart; this map holds only
// currently running requests and never becomes a second facts store.
func historyRun[T any](ctx context.Context, s *HistoryAppService, projectID, campaignID, commandID, hash string, run func(context.Context) (T, error)) (result T, err error) {
	if strings.TrimSpace(commandID) == "" || len(commandID) > 160 {
		return result, ErrHistoryInput
	}
	key := projectID + "/" + campaignID
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return result, concurrency.ErrClosed
	}
	if flight := s.flights[key]; flight != nil {
		if flight.commandID != commandID {
			s.mu.Unlock()
			return result, ErrHistoryBusy
		}
		if flight.hash != hash {
			s.mu.Unlock()
			return result, ErrHistoryCommandConflict
		}
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-flight.done:
			if flight.err != nil {
				return result, flight.err
			}
			value, ok := flight.value.(T)
			if !ok {
				return result, fmt.Errorf("history command result type mismatch")
			}
			return value, nil
		}
	}
	runCtx, cancel := context.WithCancel(ctx)
	flight := &historyFlight{commandID: commandID, hash: hash, cancel: cancel, done: make(chan struct{})}
	s.flights[key] = flight
	s.mu.Unlock()
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("history operation panicked: %v", recovered)
			slog.ErrorContext(ctx, "[history] operation panicked", "campaign_id", campaignID, "error", err)
		}
		cancel()
		s.mu.Lock()
		flight.value, flight.err = result, err
		delete(s.flights, key)
		close(flight.done)
		s.mu.Unlock()
	}()
	return run(runCtx)
}

func (s *HistoryAppService) Cancel(projectID, campaignID, commandID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	flight := s.flights[projectID+"/"+campaignID]
	if flight == nil || flight.commandID != commandID {
		return false
	}
	flight.cancel()
	return true
}

func (s *HistoryAppService) close() {
	s.mu.Lock()
	s.closed = true
	pending := make([]<-chan struct{}, 0, len(s.flights))
	for _, flight := range s.flights {
		flight.cancel()
		pending = append(pending, flight.done)
	}
	s.mu.Unlock()
	for _, done := range pending {
		<-done
	}
}
