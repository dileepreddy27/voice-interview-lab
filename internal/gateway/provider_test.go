package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestDeepgramWireContract(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Token test-placeholder" {
			t.Error("authorization header missing")
		}
		up := websocket.Upgrader{}
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer c.Close()
		kind, audio, err := c.ReadMessage()
		if err != nil || kind != websocket.BinaryMessage || len(audio) != 3200 {
			t.Error("audio framing")
		}
		c.WriteJSON(map[string]any{"type": "Results", "is_final": false, "channel": map[string]any{"alternatives": []map[string]string{{"transcript": "partial"}}}})
		c.WriteJSON(map[string]any{"type": "Results", "is_final": true, "channel": map[string]any{"alternatives": []map[string]string{{"transcript": "final answer"}}}})
		var control map[string]string
		if c.ReadJSON(&control) != nil || control["type"] != "CloseStream" {
			t.Error("missing close control")
		}
		c.WriteJSON(map[string]string{"type": "Metadata"})
	}))
	defer mock.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	p, err := newDeepgram(ctx, "test-placeholder", "ws"+strings.TrimPrefix(mock.URL, "http"))
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if err = p.Send(make([]byte, 3200)); err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"partial", "final answer"} {
		select {
		case e := <-p.Events():
			if e.Text != want || e.Final != (i == 1) {
				t.Fatal(e)
			}
		case <-ctx.Done():
			t.Fatal("timeout")
		}
	}
	if err = p.Finish(); err != nil {
		t.Fatal(err)
	}
	select {
	case _, ok := <-p.Events():
		if ok {
			t.Fatal("expected stream completion")
		}
	case <-ctx.Done():
		t.Fatal("timeout")
	}
}
