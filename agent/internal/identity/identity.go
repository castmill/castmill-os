// Package identity manages the per-device key pair and certificate (SEC-005).
//
// The key is generated on first boot and never leaves the device. In this
// version it lives on the data partition (0600, owned by the agent user);
// TPM-backed keys (SEC-006) come in Phase 2 behind the same interface.
package identity

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/castmill/castmill-os/agent/internal/config"
)

// Identity is the device key pair plus (after enrollment) its certificate.
type Identity struct {
	dir  string
	Key  crypto.Signer
	Cert *tls.Certificate // nil until enrolled
	Leaf *x509.Certificate
	// DeviceID is assigned by the backend at enrollment (empty before).
	DeviceID string
}

func (i *Identity) keyPath() string  { return filepath.Join(i.dir, "device.key") }
func (i *Identity) certPath() string { return filepath.Join(i.dir, "device.crt") }
func (i *Identity) idPath() string   { return filepath.Join(i.dir, "device-id") }

// LoadOrCreate loads the identity in dir, generating a new P-256 key if none
// exists.
func LoadOrCreate(dir string) (*Identity, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	id := &Identity{dir: dir}
	b, err := os.ReadFile(id.keyPath())
	switch {
	case errors.Is(err, os.ErrNotExist):
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, err
		}
		der, err := x509.MarshalPKCS8PrivateKey(k)
		if err != nil {
			return nil, err
		}
		pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
		if err := config.WriteFileAtomic(id.keyPath(), pemBytes, 0o600); err != nil {
			return nil, err
		}
		id.Key = k
	case err != nil:
		return nil, err
	default:
		blk, _ := pem.Decode(b)
		if blk == nil {
			return nil, fmt.Errorf("%s: no PEM data", id.keyPath())
		}
		k, err := x509.ParsePKCS8PrivateKey(blk.Bytes)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", id.keyPath(), err)
		}
		s, ok := k.(crypto.Signer)
		if !ok {
			return nil, fmt.Errorf("%s: unsupported key type", id.keyPath())
		}
		id.Key = s
	}
	if err := id.loadCert(); err != nil {
		return nil, err
	}
	return id, nil
}

func (i *Identity) loadCert() error {
	certPEM, err := os.ReadFile(i.certPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	c, leaf, err := i.pair(certPEM)
	if err != nil {
		// A certificate that doesn't match the key is useless; enroll again.
		return nil
	}
	i.Cert, i.Leaf = c, leaf
	if b, err := os.ReadFile(i.idPath()); err == nil {
		i.DeviceID = string(b)
	}
	return nil
}

func (i *Identity) pair(certPEM []byte) (*tls.Certificate, *x509.Certificate, error) {
	c := &tls.Certificate{PrivateKey: i.Key}
	for {
		var blk *pem.Block
		blk, certPEM = pem.Decode(certPEM)
		if blk == nil {
			break
		}
		if blk.Type == "CERTIFICATE" {
			c.Certificate = append(c.Certificate, blk.Bytes)
		}
	}
	if len(c.Certificate) == 0 {
		return nil, nil, errors.New("no certificate in PEM")
	}
	leaf, err := x509.ParseCertificate(c.Certificate[0])
	if err != nil {
		return nil, nil, err
	}
	pub, ok := leaf.PublicKey.(*ecdsa.PublicKey)
	own, ok2 := i.Key.Public().(*ecdsa.PublicKey)
	if !ok || !ok2 || !pub.Equal(own) {
		return nil, nil, errors.New("certificate does not match device key")
	}
	c.Leaf = leaf
	return c, leaf, nil
}

// Enrolled reports whether a valid certificate is present.
func (i *Identity) Enrolled() bool {
	return i.Cert != nil && i.DeviceID != "" && time.Now().Before(i.Leaf.NotAfter)
}

// Fingerprint is a stable short identifier derived from the public key. Shown
// on the fallback screen before a device ID exists.
func (i *Identity) Fingerprint() string {
	der, _ := x509.MarshalPKIXPublicKey(i.Key.Public())
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:8])
}

// CSR returns a PEM certificate signing request for the device key.
func (i *Identity) CSR() (string, error) {
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: "castmill-device-" + i.Fingerprint()},
	}, i.Key)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})), nil
}

// Store saves a certificate issued by the backend.
func (i *Identity) Store(deviceID, certPEM string) error {
	c, leaf, err := i.pair([]byte(certPEM))
	if err != nil {
		return err
	}
	if err := config.WriteFileAtomic(i.certPath(), []byte(certPEM), 0o644); err != nil {
		return err
	}
	if err := config.WriteFileAtomic(i.idPath(), []byte(deviceID), 0o644); err != nil {
		return err
	}
	i.Cert, i.Leaf, i.DeviceID = c, leaf, deviceID
	return nil
}

// NeedsRenewal is true when less than a third of the validity is left.
func (i *Identity) NeedsRenewal(now time.Time) bool {
	if i.Leaf == nil {
		return false
	}
	total := i.Leaf.NotAfter.Sub(i.Leaf.NotBefore)
	return i.Leaf.NotAfter.Sub(now) < total/3
}

// Forget removes the certificate (revoked device). The key is kept; a factory
// reset wipes everything.
func (i *Identity) Forget() error {
	i.Cert, i.Leaf, i.DeviceID = nil, nil, ""
	for _, p := range []string{i.certPath(), i.idPath()} {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// TLSConfig builds a client TLS configuration that trusts only roots (the
// pinned backend CA when non-nil) and presents the device certificate when
// enrolled.
func (i *Identity) TLSConfig(roots *x509.CertPool) *tls.Config {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}
	cfg.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
		if i.Cert == nil {
			return &tls.Certificate{}, nil
		}
		return i.Cert, nil
	}
	return cfg
}

// LoadRoots reads a PEM bundle; empty path returns nil (system roots).
func LoadRoots(path string, extraPEM string) (*x509.CertPool, error) {
	if path == "" && extraPEM == "" {
		return nil, nil
	}
	pool := x509.NewCertPool()
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if !pool.AppendCertsFromPEM(b) {
			return nil, fmt.Errorf("%s: no certificates", path)
		}
	}
	if extraPEM != "" && !pool.AppendCertsFromPEM([]byte(extraPEM)) {
		return nil, errors.New("backend CA override: no certificates")
	}
	return pool, nil
}
