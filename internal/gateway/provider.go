package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

type transcript struct {
	Text  string
	Final bool
	Err   error
}
type provider interface {
	Send([]byte) error
	Finish() error
	Events() <-chan transcript
	Close()
}

// Demo output is scripted. Incoming PCM is transported but never recognized.
var script = []string{
	"Our team had a customer service that slowed down during peak traffic.",
	"I was responsible for finding the bottleneck and needed a repeatable test.",
	"I measured database calls, then I implemented a cache and I tested its invalidation behavior.",
	"The result reduced repeated queries in our test. I would validate the tradeoff with production measurements.",
}

type demoProvider struct {
	bytes, segment int
	events         chan transcript
}

func newDemo() *demoProvider { return &demoProvider{events: make(chan transcript, 16)} }
func (d *demoProvider) Send(b []byte) error {
	d.bytes += len(b)
	for d.segment < len(script) && d.bytes >= (d.segment+1)*64000 {
		d.events <- transcript{Text: script[d.segment], Final: true}
		d.segment++
	}
	return nil
}
func (d *demoProvider) Finish() error             { close(d.events); return nil }
func (d *demoProvider) Events() <-chan transcript { return d.events }
func (d *demoProvider) Close()                    {}

type deepgramProvider struct {
	conn   *websocket.Conn
	events chan transcript
	ctx    context.Context
	cancel context.CancelFunc
}

func newDeepgram(ctx context.Context, key, endpoint string) (*deepgramProvider, error) {
	if key == "" {
		return nil, errors.New("live transcription is unavailable: configure DEEPGRAM_API_KEY on the server")
	}
	headers := http.Header{"Authorization": []string{"Token " + key}}
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, resp, err := dialer.DialContext(ctx, endpoint, headers)
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	if err != nil {
		return nil, errors.New("live transcription connection failed; check server credentials and connectivity")
	}
	child, cancel := context.WithCancel(ctx)
	p := &deepgramProvider{conn: conn, events: make(chan transcript, 16), ctx: child, cancel: cancel}
	conn.SetReadLimit(256 * 1024)
	go p.read()
	return p, nil
}
func (p *deepgramProvider) emit(t transcript) bool {
	select {
	case p.events <- t:
		return true
	case <-p.ctx.Done():
		return false
	}
}
func (p *deepgramProvider) read() {
	defer close(p.events)
	for {
		_, raw, err := p.conn.ReadMessage()
		if err != nil {
			if p.ctx.Err() == nil && !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				p.emit(transcript{Err: errors.New("transcription stream disconnected")})
			}
			return
		}
		var msg struct {
			Type    string `json:"type"`
			Final   bool   `json:"is_final"`
			Channel struct {
				Alternatives []struct {
					Text string `json:"transcript"`
				} `json:"alternatives"`
			} `json:"channel"`
		}
		if json.Unmarshal(raw, &msg) != nil {
			p.emit(transcript{Err: errors.New("invalid transcription response")})
			return
		}
		if msg.Type == "Error" {
			p.emit(transcript{Err: errors.New("transcription provider reported an error")})
			return
		}
		if msg.Type == "Metadata" {
			return
		}
		if msg.Type == "Results" && len(msg.Channel.Alternatives) > 0 && msg.Channel.Alternatives[0].Text != "" {
			if !p.emit(transcript{Text: msg.Channel.Alternatives[0].Text, Final: msg.Final}) {
				return
			}
		}
	}
}
func (p *deepgramProvider) Send(b []byte) error {
	p.conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	return p.conn.WriteMessage(websocket.BinaryMessage, b)
}
func (p *deepgramProvider) Finish() error {
	p.conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	return p.conn.WriteJSON(map[string]string{"type": "CloseStream"})
}
func (p *deepgramProvider) Events() <-chan transcript { return p.events }
func (p *deepgramProvider) Close()                    { p.cancel(); p.conn.Close() }
