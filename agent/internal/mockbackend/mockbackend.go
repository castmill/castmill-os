// Package mockbackend is a development implementation of the Castmill OS
// device API (docs/device-api.md). It is used by tests and to drive the
// development VM without a Castmill backend. Not for production.
package mockbackend

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/castmill/castmill-os/agent/internal/api"
)

// CA is a minimal certificate authority.
type CA struct {
	Cert *x509.Certificate
	Key  *ecdsa.PrivateKey
	PEM  []byte
}

// NewCA creates a self-signed CA.
func NewCA(name string) (*CA, error) {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial(), Subject: pkix.Name{CommonName: name},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(10, 0, 0),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &k.PublicKey, k)
	if err != nil {
		return nil, err
	}
	c, _ := x509.ParseCertificate(der)
	return &CA{Cert: c, Key: k, PEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}, nil
}

func serial() *big.Int {
	n, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	return n
}

// LoadOrCreateCA keeps a CA in dir across restarts.
func LoadOrCreateCA(dir, name string) (*CA, error) {
	certPath, keyPath := filepath.Join(dir, name+".crt"), filepath.Join(dir, name+".key")
	cb, err1 := os.ReadFile(certPath)
	kb, err2 := os.ReadFile(keyPath)
	if err1 == nil && err2 == nil {
		cblk, _ := pem.Decode(cb)
		kblk, _ := pem.Decode(kb)
		if cblk != nil && kblk != nil {
			c, err := x509.ParseCertificate(cblk.Bytes)
			if err != nil {
				return nil, err
			}
			k, err := x509.ParseECPrivateKey(kblk.Bytes)
			if err != nil {
				return nil, err
			}
			return &CA{Cert: c, Key: k, PEM: cb}, nil
		}
	}
	ca, err := NewCA("Castmill OS dev " + name)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	kder, _ := x509.MarshalECPrivateKey(ca.Key)
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kder}), 0o600); err != nil {
		return nil, err
	}
	return ca, os.WriteFile(certPath, ca.PEM, 0o644)
}

// ServerCert issues a TLS server certificate for hosts.
func (ca *CA) ServerCert(hosts []string) (tls.Certificate, error) {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial(), Subject: pkix.Name{CommonName: hosts[0]},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(1, 0, 0),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.Cert, &k.PublicKey, ca.Key)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: k}, nil
}

// SignCSR issues a device certificate with CN = deviceID.
func (ca *CA) SignCSR(csrPEM, deviceID string, validity time.Duration) (string, error) {
	blk, _ := pem.Decode([]byte(csrPEM))
	if blk == nil {
		return "", errors.New("invalid CSR PEM")
	}
	csr, err := x509.ParseCertificateRequest(blk.Bytes)
	if err != nil {
		return "", err
	}
	if err := csr.CheckSignature(); err != nil {
		return "", err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial(), Subject: pkix.Name{CommonName: deviceID, Organization: []string{"castmill-device"}},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(validity),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.Cert, csr.PublicKey, ca.Key)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), nil
}

// Options configure the server.
type Options struct {
	DeviceCA    *CA
	AutoApprove bool
	OrgToken    string // enrollments with this token are approved immediately
	BundleDir   string // served at /bundles/ (HTTP range requests supported)
	UploadDir   string
	CertTTL     time.Duration
	Log         *slog.Logger
}

// Device is the server-side record.
type Device struct {
	ID        string                  `json:"id"`
	Name      string                  `json:"name,omitempty"`
	Group     string                  `json:"group,omitempty"`
	Hardware  api.Hardware            `json:"hardware"`
	Desired   api.Desired             `json:"desired"`
	Config    api.DeviceConfig        `json:"config"`
	Status    *api.DeviceStatus       `json:"status,omitempty"`
	Telemetry *api.Telemetry          `json:"telemetry,omitempty"`
	LastSeen  time.Time               `json:"last_seen"`
	Online    bool                    `json:"online"`
	Revoked   bool                    `json:"revoked,omitempty"`
	Results   map[string]*api.Message `json:"-"`

	ws *websocket.Conn
}

type enrollment struct {
	api.Enrollment
	req api.EnrollmentRequest
}

// Server is the mock backend.
type Server struct {
	opt         Options
	mu          sync.Mutex
	enrollments map[string]*enrollment
	devices     map[string]*Device
	waiters     map[string]chan *api.Message
	seq         int
}

// New creates a server.
func New(opt Options) *Server {
	if opt.CertTTL == 0 {
		opt.CertTTL = 90 * 24 * time.Hour
	}
	if opt.Log == nil {
		opt.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Server{opt: opt, enrollments: map[string]*enrollment{}, devices: map[string]*Device{},
		waiters: map[string]chan *api.Message{}}
}

// TLSConfig returns a server TLS config that requests (but does not require)
// device certificates, so enrollment works without one.
func (s *Server) TLSConfig(serverCert tls.Certificate) *tls.Config {
	pool := x509.NewCertPool()
	pool.AddCert(s.opt.DeviceCA.Cert)
	return &tls.Config{Certificates: []tls.Certificate{serverCert}, ClientCAs: pool,
		ClientAuth: tls.VerifyClientCertIfGiven, MinVersion: tls.VersionTLS12}
}

// DeviceHandler serves the device API.
func (s *Server) DeviceHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+api.Prefix+"/enrollments", s.postEnrollment)
	mux.HandleFunc("GET "+api.Prefix+"/enrollments/{id}", s.getEnrollment)
	mux.HandleFunc("GET "+api.Prefix+"/devices/{id}/desired", s.device(func(w http.ResponseWriter, r *http.Request, d *Device) {
		writeJSON(w, 200, d.Desired)
	}))
	mux.HandleFunc("GET "+api.Prefix+"/devices/{id}/config", s.device(func(w http.ResponseWriter, r *http.Request, d *Device) {
		writeJSON(w, 200, d.Config)
	}))
	mux.HandleFunc("POST "+api.Prefix+"/devices/{id}/status", s.device(s.postStatus))
	mux.HandleFunc("POST "+api.Prefix+"/devices/{id}/certificate", s.device(s.postCertificate))
	mux.HandleFunc("POST "+api.Prefix+"/devices/{id}/uploads", s.device(s.postUpload))
	mux.HandleFunc("GET "+api.Prefix+"/devices/{id}/ws", s.device(s.websocket))
	if s.opt.BundleDir != "" {
		mux.Handle("GET /bundles/", http.StripPrefix("/bundles/", http.FileServer(http.Dir(s.opt.BundleDir))))
	}
	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func randCode() string {
	const letters = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0/O/1/I
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = letters[int(b[i])%len(letters)]
	}
	return string(b[:3]) + "-" + string(b[3:])
}

func (s *Server) postEnrollment(w http.ResponseWriter, r *http.Request) {
	var req api.EnrollmentRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil || req.CSR == "" {
		http.Error(w, "invalid enrollment request", 400)
		return
	}
	s.mu.Lock()
	s.seq++
	e := &enrollment{req: req, Enrollment: api.Enrollment{ID: fmt.Sprintf("enr-%d", s.seq), Status: api.EnrollmentPending,
		PairingCode: randCode(), PollIntervalS: 3, ExpiresAt: time.Now().Add(24 * time.Hour).UTC()}}
	s.enrollments[e.ID] = e
	auto := s.opt.AutoApprove || (s.opt.OrgToken != "" && req.OrgToken == s.opt.OrgToken)
	s.mu.Unlock()
	s.opt.Log.Info("enrollment", "id", e.ID, "pairing_code", e.PairingCode, "machine", req.Hardware.Machine)
	if auto {
		if err := s.Approve(e.PairingCode, req.Group); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
	s.mu.Lock()
	out := e.Enrollment
	s.mu.Unlock()
	writeJSON(w, 201, out)
}

func (s *Server) getEnrollment(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	e, ok := s.enrollments[r.PathValue("id")]
	var out api.Enrollment
	if ok {
		out = e.Enrollment
	}
	s.mu.Unlock()
	if !ok {
		http.Error(w, "unknown enrollment", 404)
		return
	}
	writeJSON(w, 200, out)
}

// Approve completes the enrollment with this pairing code (what an operator
// does in Castmill).
func (s *Server) Approve(code, group string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	code = strings.ToUpper(strings.TrimSpace(code))
	for _, e := range s.enrollments {
		if e.PairingCode != code || e.Status != api.EnrollmentPending {
			continue
		}
		s.seq++
		id := fmt.Sprintf("dev-%04d", s.seq)
		cert, err := s.opt.DeviceCA.SignCSR(e.req.CSR, id, s.opt.CertTTL)
		if err != nil {
			return err
		}
		e.Status, e.DeviceID, e.Certificate, e.CA = api.EnrollmentApproved, id, cert, string(s.opt.DeviceCA.PEM)
		if group == "" {
			group = e.req.Group
		}
		s.devices[id] = &Device{ID: id, Name: e.req.Name, Group: group, Hardware: e.req.Hardware,
			Config: api.DeviceConfig{Revision: 1}, Results: map[string]*api.Message{}}
		s.opt.Log.Info("device enrolled", "device_id", id, "pairing_code", code)
		return nil
	}
	return fmt.Errorf("no pending enrollment with pairing code %s", code)
}

// device authenticates the client certificate against {id}.
func (s *Server) device(h func(http.ResponseWriter, *http.Request, *Device)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 || r.TLS.PeerCertificates[0].Subject.CommonName != id {
			http.Error(w, "device certificate required", 401)
			return
		}
		s.mu.Lock()
		d, ok := s.devices[id]
		if ok {
			d.LastSeen = time.Now().UTC()
		}
		s.mu.Unlock()
		if !ok || d.Revoked {
			http.Error(w, "unknown or revoked device", 403)
			return
		}
		h(w, r, d)
	}
}

func (s *Server) postStatus(w http.ResponseWriter, r *http.Request, d *Device) {
	var st api.DeviceStatus
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&st); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.mu.Lock()
	d.Status = &st
	s.mu.Unlock()
	w.WriteHeader(204)
}

func (s *Server) postCertificate(w http.ResponseWriter, r *http.Request, d *Device) {
	var req api.CertificateRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	cert, err := s.opt.DeviceCA.SignCSR(req.CSR, d.ID, s.opt.CertTTL)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, 200, api.CertificateResponse{Certificate: cert})
}

func (s *Server) postUpload(w http.ResponseWriter, r *http.Request, d *Device) {
	kind := r.URL.Query().Get("kind")
	if kind != "logs" && kind != "screenshot" {
		http.Error(w, "kind must be logs or screenshot", 400)
		return
	}
	ext := map[string]string{"logs": ".log.gz", "screenshot": ".png"}[kind]
	s.mu.Lock()
	s.seq++
	id := fmt.Sprintf("%s-%s-%d%s", d.ID, kind, s.seq, ext)
	s.mu.Unlock()
	dir := s.opt.UploadDir
	if dir == "" {
		dir = os.TempDir()
	}
	f, err := os.Create(filepath.Join(dir, id))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer f.Close()
	if _, err := io.Copy(f, io.LimitReader(r.Body, 256<<20)); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.opt.Log.Info("upload", "device_id", d.ID, "file", f.Name())
	writeJSON(w, 201, api.UploadResponse{ID: id})
}

func (s *Server) websocket(w http.ResponseWriter, r *http.Request, d *Device) {
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	c.SetReadLimit(64 << 20)
	s.mu.Lock()
	if d.ws != nil {
		d.ws.Close(websocket.StatusPolicyViolation, "replaced by a new connection")
	}
	d.ws, d.Online = c, true
	s.mu.Unlock()
	s.opt.Log.Info("device connected", "device_id", d.ID)
	defer func() {
		s.mu.Lock()
		if d.ws == c {
			d.ws, d.Online = nil, false
		}
		s.mu.Unlock()
		s.opt.Log.Info("device disconnected", "device_id", d.ID)
	}()
	for {
		var m api.Message
		if err := wsjson.Read(r.Context(), c, &m); err != nil {
			return
		}
		s.mu.Lock()
		d.LastSeen = time.Now().UTC()
		switch m.Type {
		case api.MsgHello, api.MsgStatus:
			if m.Status != nil {
				d.Status = m.Status
			}
		case api.MsgTelemetry:
			d.Telemetry = m.Telemetry
		case api.MsgResult:
			d.Results[m.ID] = &m
			if ch, ok := s.waiters[m.ID]; ok {
				ch <- &m
				delete(s.waiters, m.ID)
			}
		}
		s.mu.Unlock()
	}
}

func (s *Server) send(id string, m *api.Message) error {
	s.mu.Lock()
	d, ok := s.devices[id]
	var c *websocket.Conn
	if ok {
		c = d.ws
	}
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("unknown device %s", id)
	}
	if c == nil {
		return fmt.Errorf("device %s is offline", id)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return wsjson.Write(ctx, c, m)
}

// Command sends a command and waits for the result.
func (s *Server) Command(ctx context.Context, deviceID, name string, args map[string]any) (*api.Message, error) {
	s.mu.Lock()
	s.seq++
	cid := fmt.Sprintf("cmd-%d", s.seq)
	ch := make(chan *api.Message, 1)
	s.waiters[cid] = ch
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.waiters, cid); s.mu.Unlock() }()
	if err := s.send(deviceID, &api.Message{Type: api.MsgCommand, ID: cid, Command: name, Args: args}); err != nil {
		return nil, err
	}
	select {
	case m := <-ch:
		return m, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// SetDesired updates the desired version and notifies the device if online.
func (s *Server) SetDesired(deviceID string, d api.Desired) error {
	s.mu.Lock()
	dev, ok := s.devices[deviceID]
	if ok {
		dev.Desired = d
	}
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("unknown device %s", deviceID)
	}
	_ = s.send(deviceID, &api.Message{Type: api.MsgDesired, Desired: &d})
	return nil
}

// SetConfig updates the device configuration and pushes it.
func (s *Server) SetConfig(deviceID string, c api.DeviceConfig) error {
	s.mu.Lock()
	dev, ok := s.devices[deviceID]
	if ok {
		c.Revision = dev.Config.Revision + 1
		dev.Config = c
	}
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("unknown device %s", deviceID)
	}
	_ = s.send(deviceID, &api.Message{Type: api.MsgConfig, Config: &c})
	return nil
}

// Revoke rejects the device from now on.
func (s *Server) Revoke(deviceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d, ok := s.devices[deviceID]; ok {
		d.Revoked = true
		if d.ws != nil {
			d.ws.Close(websocket.StatusPolicyViolation, "revoked")
		}
	}
}

// Devices returns a snapshot.
func (s *Server) Devices() []Device {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Device, 0, len(s.devices))
	for _, d := range s.devices {
		out = append(out, *d)
	}
	return out
}

// Enrollments returns pending enrollments.
func (s *Server) Enrollments() []api.Enrollment {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []api.Enrollment
	for _, e := range s.enrollments {
		if e.Status == api.EnrollmentPending {
			x := e.Enrollment
			out = append(out, x)
		}
	}
	return out
}

// AdminHandler is a small JSON API for developers (bind it to localhost):
//
//	GET  /devices
//	GET  /enrollments
//	POST /pair                 {"code":"ABC-123","group":"..."}
//	POST /devices/{id}/desired {"version":"...","bundle_url":"...","activate_policy":"now"}
//	POST /devices/{id}/config  {DeviceConfig}
//	POST /devices/{id}/command {"command":"screenshot","args":{...}}
//	POST /devices/{id}/revoke
func (s *Server) AdminHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /devices", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, s.Devices()) })
	mux.HandleFunc("GET /enrollments", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, s.Enrollments()) })
	mux.HandleFunc("POST /pair", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Code, Group string }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if err := s.Approve(req.Code, req.Group); err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /devices/{id}/desired", func(w http.ResponseWriter, r *http.Request) {
		var d api.Desired
		if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if err := s.SetDesired(r.PathValue("id"), d); err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /devices/{id}/config", func(w http.ResponseWriter, r *http.Request) {
		var c api.DeviceConfig
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if err := s.SetConfig(r.PathValue("id"), c); err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /devices/{id}/command", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Command string         `json:"command"`
			Args    map[string]any `json:"args"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()
		res, err := s.Command(ctx, r.PathValue("id"), req.Command, req.Args)
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		writeJSON(w, 200, res)
	})
	mux.HandleFunc("POST /devices/{id}/revoke", func(w http.ResponseWriter, r *http.Request) {
		s.Revoke(r.PathValue("id"))
		w.WriteHeader(204)
	})
	return mux
}
