package codex

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestProtocolCancelSettlesRejectedProviderRequests(t *testing.T) {
	for _, method := range []string{
		"item/commandExecution/requestApproval",
		"item/fileChange/requestApproval",
		"item/permissions/requestApproval",
		"item/tool/requestUserInput",
	} {
		t.Run(method, func(t *testing.T) {
			fixture := newProtocolFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			host := &testHost{started: make(chan struct{}, 1), answer: make(chan struct{})}
			done := startFixtureTurn(t, ctx, fixture, host)
			fixture.send(t, `1`, method, map[string]any{"threadId": "thread", "turnId": "turn"})
			cancel()
			// A rejection already sent before cancellation may precede interrupt.
			// Both protocol obligations must settle, in either legitimate order.
			interrupted, rejected := false, false
			for range 2 {
				msg := fixture.receive(t)
				if msg.Method == "turn/interrupt" {
					if interrupted {
						t.Fatal("duplicate turn interruption")
					}
					interrupted = true
					if err := json.NewEncoder(fixture.server).Encode(packet{ID: msg.ID, Result: json.RawMessage(`{}`)}); err != nil {
						t.Fatal(err)
					}
					continue
				}
				if string(msg.ID) != "1" || rejected {
					t.Fatalf("unexpected rejection response: %+v", msg)
				}
				rejected = true
				if method == "item/tool/requestUserInput" {
					if msg.Error == nil || msg.Error.Code != -32601 {
						t.Fatalf("native question was not refused: %+v", msg)
					}
				} else {
					want := `{"decision":"decline"}`
					if method == "item/permissions/requestApproval" {
						want = `{"permissions":{},"scope":"turn"}`
					}
					if msg.Error != nil || string(msg.Result) != want {
						t.Fatalf("provider permission was not refused: %+v", msg)
					}
				}
			}
			if !interrupted || !rejected {
				t.Fatal("cancellation lost a protocol obligation")
			}
			select {
			case <-done:
				t.Fatal("pause settled without provider terminal acknowledgement")
			default:
			}
			fixture.send(t, "", "turn/completed", map[string]any{"threadId": "thread", "turn": map[string]any{"id": "turn", "status": "interrupted"}})
			select {
			case result := <-done:
				if !errors.Is(result.err, context.Canceled) || !result.result.Settled || host.calls.Load() != 0 {
					t.Fatalf("rejection cancellation did not settle safely: %+v calls=%d", result, host.calls.Load())
				}
			case <-time.After(time.Second):
				t.Fatal("rejection cancellation did not finish")
			}
		})
	}
}
