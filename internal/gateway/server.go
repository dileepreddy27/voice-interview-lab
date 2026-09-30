package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const maxAudio = 120 * 32000
const deepgramEndpoint = "wss://api.deepgram.com/v1/listen?model=nova-3&encoding=linear16&sample_rate=16000&channels=1&interim_results=true&smart_format=true"

type Server struct {
	CoachURL, Key, WebDir string
	active                atomic.Int32
	client                *http.Client
}

func New(coachURL, key, webDir string) *Server {
	return &Server{CoachURL: coachURL, Key: key, WebDir: webDir, client: &http.Client{Timeout: 3 * time.Second}}
}
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	} // CLI clients; local-only service, not authentication.
	u, err := url.Parse(origin)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host == r.Host
}
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"status":"ok"}`)
	})
	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"live_available": s.Key != "", "max_seconds": 120})
	})
	mux.HandleFunc("/ws", s.session)
	mux.Handle("/", http.FileServer(http.Dir(s.WebDir)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; worker-src 'self'; frame-ancestors 'none'")
		mux.ServeHTTP(w, r)
	})
}

type input struct {
	kind int
	data []byte
	err  error
}

func readInput(ctx context.Context, c *websocket.Conn, ch chan<- input) {
	for {
		kind, data, err := c.ReadMessage()
		select {
		case ch <- input{kind, data, err}:
		case <-ctx.Done():
			return
		}
		if err != nil {
			return
		}
	}
}
func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	if s.active.Add(1) > 16 {
		s.active.Add(-1)
		http.Error(w, "session capacity reached", 503)
		return
	}
	defer s.active.Add(-1)
	upgrader := websocket.Upgrader{CheckOrigin: sameOrigin, HandshakeTimeout: 5 * time.Second}
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()
	c.SetReadLimit(32000)
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	send := func(v any) error { c.SetWriteDeadline(time.Now().Add(3 * time.Second)); return c.WriteJSON(v) }
	fail := func(code, msg string) { send(map[string]any{"type": "error", "code": code, "message": msg}) }
	var hello struct {
		Type       string `json:"type"`
		Source     string `json:"source"`
		Consent    bool   `json:"consent"`
		SampleRate int    `json:"sample_rate"`
	}
	if c.ReadJSON(&hello) != nil || hello.Type != "start" || !hello.Consent || hello.SampleRate != 16000 || (hello.Source != "demo" && hello.Source != "microphone") {
		fail("invalid_start", "Consent, source, and 16000 Hz PCM are required.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 125*time.Second)
	defer cancel()
	var p provider
	if hello.Source == "demo" {
		p = newDemo()
	} else {
		p, err = newDeepgram(ctx, s.Key, deepgramEndpoint)
		if err != nil {
			fail("provider_unavailable", err.Error())
			return
		}
	}
	defer p.Close()
	c.SetReadDeadline(time.Now().Add(120 * time.Second))
	if send(map[string]any{"type": "ready", "source": hello.Source, "synthetic": hello.Source == "demo", "format": "pcm_s16le/16000/mono"}) != nil {
		return
	}
	incoming := make(chan input, 8)
	go readInput(ctx, c, incoming)
	started := time.Now()
	total, seq := 0, 0
	stopping := false
	var finalized []string
	textLength := 0
	var drain <-chan time.Time
	handle := func(t transcript) error {
		if t.Err != nil {
			return t.Err
		}
		if t.Final {
			textLength += len(t.Text)
			if textLength > 24000 {
				return errors.New("transcript length limit reached")
			}
			finalized = append(finalized, t.Text)
		}
		return send(map[string]any{"type": "transcript", "text": t.Text, "final": t.Final, "synthetic": hello.Source == "demo"})
	}
	complete := func() {
		if total == 0 {
			fail("empty_audio", "No audio received.")
			return
		}
		elapsed := time.Since(started).Milliseconds()
		duration := float64(total) / 32000
		payload, _ := json.Marshal(map[string]any{"text": strings.Join(finalized, " "), "duration_seconds": duration})
		req, err := http.NewRequestWithContext(ctx, "POST", s.CoachURL+"/evaluate", bytes.NewReader(payload))
		if err != nil {
			fail("coach_unavailable", "Coaching service address is invalid.")
			return
		}
		req.Header.Set("Content-Type", "application/json")
		before := time.Now()
		resp, err := s.client.Do(req)
		if err != nil {
			fail("coach_unavailable", "Audio processed, but coaching service is unavailable.")
			return
		}
		defer resp.Body.Close()
		var feedback map[string]any
		if resp.StatusCode != 200 || json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&feedback) != nil {
			fail("coach_unavailable", "Coaching service returned an invalid response.")
			return
		}
		send(map[string]any{"type": "complete", "synthetic": hello.Source == "demo", "feedback": feedback, "telemetry": map[string]any{"chunks": seq, "audio_bytes": total, "audio_seconds": duration, "session_elapsed_ms": elapsed, "coach_roundtrip_ms": time.Since(before).Milliseconds()}})
	}
	for {
		select {
		case <-ctx.Done():
			fail("timeout", "Session time limit reached.")
			return
		case <-drain:
			fail("provider_timeout", "Transcription did not finish within five seconds.")
			return
		case t, ok := <-p.Events():
			if !ok {
				if !stopping {
					fail("provider_closed", "Transcription stream ended unexpectedly.")
				} else {
					complete()
				}
				return
			}
			if err := handle(t); err != nil {
				fail("provider_error", err.Error())
				return
			}
		case in := <-incoming:
			if in.err != nil {
				return
			}
			if stopping {
				fail("invalid_state", "Audio cannot be sent after stop.")
				return
			}
			if in.kind == websocket.TextMessage {
				var cmd struct {
					Type string `json:"type"`
				}
				if json.Unmarshal(in.data, &cmd) != nil || cmd.Type != "stop" {
					fail("invalid_control", "Expected stop control message.")
					return
				}
				stopping = true
				if p.Finish() != nil {
					fail("provider_error", "Could not finalize transcription.")
					return
				}
				timer := time.NewTimer(5 * time.Second)
				defer timer.Stop()
				drain = timer.C
				continue
			}
			if in.kind != websocket.BinaryMessage || len(in.data) == 0 || len(in.data)%2 != 0 || total+len(in.data) > maxAudio {
				fail("invalid_audio", "Expected even-sized PCM chunks within the 120-second audio limit.")
				return
			}
			total += len(in.data)
			seq++
			if p.Send(in.data) != nil {
				fail("provider_error", "Audio forwarding failed.")
				return
			}
			if send(map[string]any{"type": "ack", "seq": seq, "audio_bytes": total, "audio_seconds": float64(total) / 32000}) != nil {
				return
			}
		}
	}
}
