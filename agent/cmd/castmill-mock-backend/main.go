// Command castmill-mock-backend runs a development implementation of the
// Castmill OS device API, for the development VM and local testing.
//
//	go run ./cmd/castmill-mock-backend --state .mock --host 10.0.2.2 --auto-approve
//
// The device API listens on --listen (HTTPS, mTLS). A JSON admin API for you
// listens on --admin (plain HTTP, localhost only by default). See
// docs/device-api.md for both.
package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/castmill/castmill-os/agent/internal/mockbackend"
)

func main() {
	// QEMU user networking reaches the host's loopback as 10.0.2.2.
	listen := flag.String("listen", "127.0.0.1:8443", "device API address (HTTPS)")
	admin := flag.String("admin", "127.0.0.1:8081", "admin API address (HTTP)")
	stateDir := flag.String("state", ".mock-backend", "directory for CAs and uploads")
	hosts := flag.String("host", "localhost,127.0.0.1,10.0.2.2", "comma-separated names/IPs for the server certificate (10.0.2.2 = host as seen from QEMU)")
	auto := flag.Bool("auto-approve", false, "approve every enrollment immediately")
	token := flag.String("org-token", "", "enrollments with this org_token are approved immediately")
	bundles := flag.String("bundles", "", "directory served at /bundles/ (RAUC bundles)")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	serverCA, err := mockbackend.LoadOrCreateCA(*stateDir, "server-ca")
	if err != nil {
		log.Error("server CA", "err", err)
		os.Exit(1)
	}
	deviceCA, err := mockbackend.LoadOrCreateCA(*stateDir, "device-ca")
	if err != nil {
		log.Error("device CA", "err", err)
		os.Exit(1)
	}
	cert, err := serverCA.ServerCert(strings.Split(*hosts, ","))
	if err != nil {
		log.Error("server certificate", "err", err)
		os.Exit(1)
	}
	uploads := filepath.Join(*stateDir, "uploads")
	_ = os.MkdirAll(uploads, 0o755)
	s := mockbackend.New(mockbackend.Options{DeviceCA: deviceCA, AutoApprove: *auto, OrgToken: *token,
		BundleDir: *bundles, UploadDir: uploads, Log: log})

	go func() {
		log.Info("admin API", "addr", "http://"+*admin)
		if err := http.ListenAndServe(*admin, s.AdminHandler()); err != nil {
			log.Error("admin API", "err", err)
			os.Exit(1)
		}
	}()
	srv := &http.Server{Addr: *listen, Handler: s.DeviceHandler(), TLSConfig: s.TLSConfig(cert)}
	log.Info("device API", "addr", "https://"+*listen,
		"server_ca", filepath.Join(*stateDir, "server-ca.crt"), "hint", "put the server CA in castmill.conf [backend] ca_cert_file (dev images)")
	if err := srv.ListenAndServeTLS("", ""); err != nil {
		log.Error("device API", "err", err)
		os.Exit(1)
	}
}
