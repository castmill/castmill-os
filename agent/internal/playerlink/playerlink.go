// Package playerlink is the local link between the agent and the player
// instances (one Electron process per output).
//
// Transport: Unix stream socket, one JSON object per line, both directions.
//
// Player -> agent:
//
//	{"type":"hello","output":"HDMI-A-1"}
//	{"type":"frame","output":"HDMI-A-1"}             frame-rendered heartbeat
//	{"type":"capabilities","data":{...}}             GPU / decode info (MED-007)
//	{"type":"stats","data":{"dropped_frames":0}}     playback telemetry
//	{"type":"screenshot","id":"...","png":"<b64>"}   answer to a screenshot request
//
// Agent -> player:
//
//	{"type":"state","state":{...}}                   enrollment / pairing / URL
//	{"type":"reload"}
//	{"type":"screenshot","id":"..."}
package playerlink

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"
)

// PlayerState is pushed to every player.
type PlayerState struct {
	Enrolled     bool   `json:"enrolled"`
	DeviceID     string `json:"device_id,omitempty"`
	Fingerprint  string `json:"fingerprint"`
	PairingCode  string `json:"pairing_code,omitempty"`
	PlayerURL    string `json:"player_url,omitempty"`
	DevImage     bool   `json:"dev_image"`
	OSVersion    string `json:"os_version"`
	BackendError string `json:"backend_error,omitempty"`
	Online       bool   `json:"online"`
}

type msg struct {
	Type   string          `json:"type"`
	Output string          `json:"output,omitempty"`
	ID     string          `json:"id,omitempty"`
	PNG    string          `json:"png,omitempty"`
	Data   json.RawMessage `json:"data,omitempty"`
	State  *PlayerState    `json:"state,omitempty"`
}

type conn struct {
	c      net.Conn
	output string
	wmu    sync.Mutex
}

func (c *conn) send(m msg) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_ = c.c.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err = c.c.Write(append(b, '\n'))
	return err
}

// Server accepts player connections.
type Server struct {
	log    *slog.Logger
	mu     sync.Mutex
	ln     net.Listener
	byOut  map[string]*conn
	frames map[string]time.Time
	caps   json.RawMessage
	stats  map[string]json.RawMessage
	state  PlayerState
	shots  map[string]chan []byte
}

// Listen creates the socket at path (mode 0660; the directory restricts access
// to the castmill group).
func Listen(path string, log *slog.Logger) (*Server, error) {
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o660); err != nil {
		ln.Close()
		return nil, err
	}
	return &Server{log: log, ln: ln, byOut: map[string]*conn{}, frames: map[string]time.Time{},
		stats: map[string]json.RawMessage{}, shots: map[string]chan []byte{}}, nil
}

// Serve accepts connections until ctx is done.
func (s *Server) Serve(ctx context.Context) {
	go func() { <-ctx.Done(); s.ln.Close() }()
	for {
		c, err := s.ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			s.log.Warn("player socket accept", "err", err)
			time.Sleep(time.Second)
			continue
		}
		go s.handle(&conn{c: c})
	}
}

func (s *Server) handle(c *conn) {
	defer c.c.Close()
	sc := bufio.NewScanner(c.c)
	sc.Buffer(make([]byte, 64<<10), 32<<20) // screenshots are large
	for sc.Scan() {
		var m msg
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			s.log.Warn("player: bad message", "err", err)
			continue
		}
		s.dispatch(c, m)
	}
	s.mu.Lock()
	if c.output != "" && s.byOut[c.output] == c {
		delete(s.byOut, c.output)
		s.log.Info("player disconnected", "output", c.output)
	}
	s.mu.Unlock()
}

func (s *Server) dispatch(c *conn, m msg) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch m.Type {
	case "hello":
		if m.Output == "" {
			return
		}
		c.output = m.Output
		s.byOut[m.Output] = c
		s.log.Info("player connected", "output", m.Output)
		st := s.state
		go func() { _ = c.send(msg{Type: "state", State: &st}) }()
	case "frame":
		out := m.Output
		if out == "" {
			out = c.output
		}
		if out != "" {
			s.frames[out] = time.Now()
		}
	case "capabilities":
		s.caps = m.Data
	case "stats":
		if c.output != "" {
			s.stats[c.output] = m.Data
		}
	case "screenshot":
		if ch, ok := s.shots[m.ID]; ok {
			png, err := base64.StdEncoding.DecodeString(m.PNG)
			if err != nil || len(png) == 0 {
				png = nil // reported as an error to the backend
			}
			ch <- png
			delete(s.shots, m.ID)
		}
	}
}

// SetState updates and broadcasts the player state.
func (s *Server) SetState(st PlayerState) {
	s.mu.Lock()
	changed := st != s.state
	s.state = st
	conns := s.conns()
	s.mu.Unlock()
	if !changed {
		return
	}
	for _, c := range conns {
		_ = c.send(msg{Type: "state", State: &st})
	}
}

func (s *Server) conns() []*conn {
	out := make([]*conn, 0, len(s.byOut))
	for _, c := range s.byOut {
		out = append(out, c)
	}
	return out
}

// Outputs returns the attached outputs and their last frame time.
func (s *Server) Outputs() map[string]time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]time.Time{}
	for name := range s.byOut {
		out[name] = s.frames[name]
	}
	return out
}

// LastFrame returns the last frame heartbeat of an output (attached or not).
func (s *Server) LastFrame(output string) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.frames[output]
}

// Capabilities returns the last decode/GPU capabilities reported by a player.
func (s *Server) Capabilities() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.caps) == 0 {
		return nil
	}
	var m map[string]any
	_ = json.Unmarshal(s.caps, &m)
	return m
}

// Stats returns per-output playback stats.
func (s *Server) Stats() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]any{}
	for k, v := range s.stats {
		var m any
		if json.Unmarshal(v, &m) == nil {
			out[k] = m
		}
	}
	return out
}

// Reload asks the player on output to reload its window.
func (s *Server) Reload(output string) error {
	s.mu.Lock()
	c := s.byOut[output]
	s.mu.Unlock()
	if c == nil {
		return fmt.Errorf("no player on output %s", output)
	}
	return c.send(msg{Type: "reload"})
}

// Screenshot requests a PNG of output's window.
func (s *Server) Screenshot(ctx context.Context, output string) ([]byte, error) {
	id := fmt.Sprintf("%d", time.Now().UnixNano())
	ch := make(chan []byte, 1)
	s.mu.Lock()
	c := s.byOut[output]
	if c != nil {
		s.shots[id] = ch
	}
	s.mu.Unlock()
	if c == nil {
		return nil, fmt.Errorf("no player on output %s", output)
	}
	defer func() { s.mu.Lock(); delete(s.shots, id); s.mu.Unlock() }()
	if err := c.send(msg{Type: "screenshot", ID: id}); err != nil {
		return nil, err
	}
	select {
	case png := <-ch:
		if png == nil {
			return nil, errors.New("player returned an invalid screenshot")
		}
		return png, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
