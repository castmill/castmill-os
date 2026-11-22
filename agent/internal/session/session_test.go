package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/castmill/castmill-os/agent/internal/api"
)

func TestConnectors(t *testing.T) {
	root := t.TempDir()
	for _, c := range []string{"card0-HDMI-A-1", "card0-HDMI-A-2", "card1-DP-1", "card1-bad name"} {
		os.MkdirAll(filepath.Join(root, "sys/class/drm", c), 0o755)
		os.WriteFile(filepath.Join(root, "sys/class/drm", c, "status"), []byte("disconnected\n"), 0o644)
	}
	os.MkdirAll(filepath.Join(root, "sys/class/drm/renderD128"), 0o755)
	got := strings.Join(Connectors(root), ",")
	if got != "DP-1,HDMI-A-1,HDMI-A-2" {
		t.Fatalf("got %s", got)
	}
}

func TestWestonINI(t *testing.T) {
	ini := WestonINI([]string{"HDMI-A-1", "HDMI-A-2", "DP-1"}, map[string]api.OutputConfig{
		"HDMI-A-1": {Mode: "1920x1080@60", Transform: "rotate-90", Scale: 1},
		"HDMI-A-2": {Mode: "1920x1080\nname=evil", Transform: "sideways"},
		"DP-1":     {Disabled: true},
	})
	for _, want := range []string{
		"shell=kiosk", "idle-time=0", "require-input=false",
		"[output]\nname=HDMI-A-1\nmode=1920x1080@60\ntransform=rotate-90\nscale=1\napp-ids=castmill-output-HDMI-A-1\n",
		"[output]\nname=HDMI-A-2\nmode=preferred\ntransform=normal\napp-ids=castmill-output-HDMI-A-2\n",
		"name=DP-1\nmode=off\n",
	} {
		if !strings.Contains(ini, want) {
			t.Errorf("missing %q in:\n%s", want, ini)
		}
	}
	if strings.Contains(ini, "evil") {
		t.Fatal("unvalidated value written to weston.ini")
	}
}

func TestDisabled(t *testing.T) {
	got := Disabled(map[string]api.OutputConfig{"DP-1": {Disabled: true}, "HDMI-A-1": {}, "x y": {Disabled: true}})
	if len(got) != 1 || got[0] != "DP-1" {
		t.Fatalf("got %v", got)
	}
}
