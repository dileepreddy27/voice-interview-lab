package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func testServer(t *testing.T, coach string) (*httptest.Server, *Server) {
	t.Helper()
	s := New(coach, "", "../../web")
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts, s
}
func connect(t *testing.T, ts *httptest.Server) *websocket.Conn {
	t.Helper()
	c, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(ts.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	c.SetReadDeadline(time.Now().Add(10 * time.Second))
	return c
}
func start(t *testing.T, c *websocket.Conn, source string, consent bool) map[string]any {
	t.Helper()
	if err := c.WriteJSON(map[string]any{"type": "start", "source": source, "consent": consent, "sample_rate": 16000}); err != nil {
		t.Fatal(err)
	}
	return event(t, c)
}
func event(t *testing.T, c *websocket.Conn) map[string]any {
	t.Helper()
	var data map[string]any
	if err := c.ReadJSON(&data); err != nil {
		t.Fatal(err)
	}
	return data
}
func TestConsentAndCredentials(t *testing.T) {
	ts, _ := testServer(t, "http://unused")
	for _, tc := range []struct {
		source  string
		consent bool
		code    string
	}{{"demo", false, "invalid_start"}, {"microphone", true, "provider_unavailable"}, {"unknown", true, "invalid_start"}} {
		t.Run(tc.code+tc.source, func(t *testing.T) {
			c := connect(t, ts)
			m := start(t, c, tc.source, tc.consent)
			if m["code"] != tc.code {
				t.Fatal(m)
			}
		})
	}
}
func TestAudioValidation(t *testing.T) {
	ts, _ := testServer(t, "http://unused")
	for _, payload := range [][]byte{{}, {1}, make([]byte, 32001)} {
		c := connect(t, ts)
		start(t, c, "demo", true)
		c.WriteMessage(websocket.BinaryMessage, payload)
		// Oversized frames are closed by the WebSocket library before decoding.
		if len(payload) <= 32000 {
			if m := event(t, c); m["code"] != "invalid_audio" {
				t.Fatal(m)
			}
		} else {
			if _, _, err := c.ReadMessage(); err == nil {
				t.Fatal("oversize accepted")
			}
		}
		c.Close()
	}
}
func TestOrigin(t *testing.T) {
	ts, _ := testServer(t, "")
	_, r, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(ts.URL, "http")+"/ws", http.Header{"Origin": []string{"https://untrusted.example"}})
	if err == nil || r.StatusCode != 403 {
		t.Fatal("cross-origin accepted")
	}
	r.Body.Close()
}
func TestDemoEndToEnd(t *testing.T) {
	coach := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Text     string  `json:"text"`
			Duration float64 `json:"duration_seconds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Duration != 8 || !strings.Contains(body.Text, "production measurements") {
			t.Error(body)
		}
		fmt.Fprint(w, `{"engine":"test","word_count":71}`)
	}))
	defer coach.Close()
	ts, _ := testServer(t, coach.URL)
	c := connect(t, ts)
	if m := start(t, c, "demo", true); m["synthetic"] != true {
		t.Fatal(m)
	}
	finals, acks := 0, 0
	for i := 0; i < 80; i++ {
		if err := c.WriteMessage(websocket.BinaryMessage, make([]byte, 3200)); err != nil {
			t.Fatal(err)
		}
	}
	c.WriteJSON(map[string]string{"type": "stop"})
	for {
		m := event(t, c)
		switch m["type"] {
		case "ack":
			acks++
		case "transcript":
			if m["final"] == true {
				finals++
			}
		case "complete":
			if finals != 4 || acks != 80 {
				t.Fatalf("finals=%d acks=%d", finals, acks)
			}
			tel := m["telemetry"].(map[string]any)
			if tel["audio_bytes"] != float64(256000) {
				t.Fatal(tel)
			}
			return
		case "error":
			t.Fatal(m)
		}
	}
}
func TestCoachOutage(t *testing.T) {
	ts, _ := testServer(t, "http://127.0.0.1:1")
	c := connect(t, ts)
	start(t, c, "demo", true)
	c.WriteMessage(websocket.BinaryMessage, make([]byte, 3200))
	event(t, c)
	c.WriteJSON(map[string]string{"type": "stop"})
	if m := event(t, c); m["code"] != "coach_unavailable" {
		t.Fatal(m)
	}
}
func TestCapacityAndRelease(t *testing.T) {
	ts, s := testServer(t, "")
	var connections []*websocket.Conn
	for i := 0; i < 16; i++ {
		c := connect(t, ts)
		start(t, c, "demo", true)
		connections = append(connections, c)
	}
	_, r, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(ts.URL, "http")+"/ws", nil)
	if err == nil || r.StatusCode != 503 {
		t.Fatal("capacity limit not enforced")
	}
	r.Body.Close()
	var wg sync.WaitGroup
	for _, c := range connections {
		wg.Add(1)
		go func(c *websocket.Conn) { defer wg.Done(); c.Close() }(c)
	}
	wg.Wait()
	deadline := time.Now().Add(time.Second)
	for s.active.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if s.active.Load() != 0 {
		t.Fatal("sessions leaked")
	}
}
