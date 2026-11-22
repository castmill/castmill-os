// Package playerlink is the local link between the agent and the player
// instances (one kiosk-shell process per output).
//
// Transport: Unix stream socket, one JSON object per line, both directions.
//
// Player -> agent:
//
//	{"type":"hello","output":"HDMI-A-1"}
//	{"type":"frame","output":"HDMI-A-1"}                    frame-rendered heartbeat
//	{"type":"call","id":"7","method":"telemetry","args":{}}  native Machine call
//
// Agent -> player:
//
//	{"type":"state","state":{...}}                          player URL, device facts
//	{"type":"reload"}
//	{"type":"reply","id":"7","ok":true,"data":{...}}         answer to a call
//
// Calls come from the Castmill device app through the kiosk shell's
// castmillOS bridge (docs/castmill-integration.md).
package playerlink

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"
)

// PlayerState is pushed to every player.
type PlayerState struct {
	PlayerURL    string `json:"player_url,omitempty"`
	DevImage     bool   `json:"dev_image"`
	OSVersion    string `json:"os_version"`
	DeviceName   string `json:"device_name"` // short local identifier for support
	BackendError string `json:"backend_error,omitempty"`
	Online       bool   `json:"online"`
}

// Handler answers a call from the player on output.
type Handler func(ctx context.Context, output, method string, args json.RawMessage) (any, error)

type msg struct {
	Type   string          `json:"type"`
	Output string          `json:"output,omitempty"`
	ID     string          `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Args   json.RawMessage `json:"args,omitempty"`
	OK     bool            `json:"ok,omitempty"`
	Data   any             `json:"data,omitempty"`
	Error  string          `json:"error,omitempty"`
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
	log     *slog.Logger
	handler Handler
	mu      sync.Mutex
	ln      net.Listener
	byOut   map[string]*conn
	frames  map[string]time.Time
	state   PlayerState
}

// Listen creates the socket at path (mode 0660; the directory restricts access
// to the castmill group).
func Listen(path string, handler Handler, log *slog.Logger) (*Server, error) {
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o660); err != nil {
		ln.Close()
		return nil, err
	}
	return &Server{log: log, handler: handler, ln: ln, byOut: map[string]*conn{}, frames: map[string]time.Time{}}, nil
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
		go s.handle(ctx, &conn{c: c})
	}
}

func (s *Server) handle(ctx context.Context, c *conn) {
	defer c.c.Close()
	sc := bufio.NewScanner(c.c)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		var m msg
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			s.log.Warn("player: bad message", "err", err)
			continue
		}
		s.dispatch(ctx, c, m)
	}
	s.mu.Lock()
	if c.output != "" && s.byOut[c.output] == c {
		delete(s.byOut, c.output)
		s.log.Info("player disconnected", "output", c.output)
	}
	s.mu.Unlock()
}

func (s *Server) dispatch(ctx context.Context, c *conn, m msg) {
	switch m.Type {
	case "hello":
		if m.Output == "" {
			return
		}
		s.mu.Lock()
		c.output = m.Output
		s.byOut[m.Output] = c
		st := s.state
		s.mu.Unlock()
		s.log.Info("player connected", "output", m.Output)
		_ = c.send(msg{Type: "state", State: &st})
	case "frame":
		out := m.Output
		if out == "" {
			out = c.output
		}
		if out != "" {
			s.mu.Lock()
			s.frames[out] = time.Now()
			s.mu.Unlock()
		}
	case "call":
		if c.output == "" || s.handler == nil {
			_ = c.send(msg{Type: "reply", ID: m.ID, Error: "say hello first"})
			return
		}
		go func() {
			cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			data, err := s.handler(cctx, c.output, m.Method, m.Args)
			r := msg{Type: "reply", ID: m.ID, OK: err == nil, Data: data}
			if err != nil {
				r.Error = err.Error()
			}
			if err := c.send(r); err != nil {
				s.log.Warn("player: reply", "err", err)
			}
		}()
	}
}

// SetState updates and broadcasts the player state.
func (s *Server) SetState(st PlayerState) {
	s.mu.Lock()
	changed := st != s.state
	s.state = st
	conns := make([]*conn, 0, len(s.byOut))
	for _, c := range s.byOut {
		conns = append(conns, c)
	}
	s.mu.Unlock()
	if !changed {
		return
	}
	for _, c := range conns {
		_ = c.send(msg{Type: "state", State: &st})
	}
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
