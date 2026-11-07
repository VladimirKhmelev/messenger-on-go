package ws

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gorilla/websocket"
)

func TestWriteGateWaitIsCancelable(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newSession("u", "", nil, nil, nil)
		release := make(chan struct{})
		done := make(chan error, 1)
		go func() { done <- s.writeFrame(context.Background(), func(time.Time) error { <-release; return nil }) }()
		synctest.Wait()
		ctx, cancel := context.WithCancel(t.Context())
		waiting := make(chan error, 1)
		go func() {
			waiting <- s.writeFrame(ctx, func(time.Time) error { t.Error("canceled waiter acquired writer"); return nil })
		}()
		synctest.Wait()
		cancel()
		if err := <-waiting; !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		close(release)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	})
}

// A real peer that never reads must not retain a fanout worker indefinitely.
func TestBlockedWebSocketWriteStops(t *testing.T) {
	for _, mode := range []string{"deadline", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			accepted := make(chan *websocket.Conn, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					return
				}
				accepted <- c
			}))
			defer srv.Close()
			peer, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = peer.Close() }()
			c := <-accepted
			defer func() { _ = c.Close() }()
			s := newSession("u", "", c, nil, nil)
			ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
			defer cancel()
			payload := make([]byte, 16<<20)
			started := make(chan struct{})
			done := make(chan error, 1)
			go func() {
				done <- s.writeFrame(ctx, func(deadline time.Time) error {
					if err := c.SetWriteDeadline(deadline); err != nil {
						return err
					}
					close(started)
					return c.WriteMessage(websocket.BinaryMessage, payload)
				})
			}()
			<-started
			if mode == "cancel" {
				cancel()
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("blocked write succeeded")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("write did not stop")
			}
			if err := s.writeContext(t.Context(), serverMessage{Type: "after_failure"}); err == nil {
				t.Fatal("partially written socket remained usable")
			}
		})
	}
}
