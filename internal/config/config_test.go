package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// write puts content in a temporary file and returns its path.
func write(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}

func TestLoadMasterDefaults(t *testing.T) {
	path := write(t, "master.toml", `secret = "hunter2"`)

	cfg, err := LoadMaster(path)
	if err != nil {
		t.Fatalf("LoadMaster: %v", err)
	}
	if cfg.Listen != DefaultListen {
		t.Errorf("listen = %q, want %q", cfg.Listen, DefaultListen)
	}
	if cfg.ReportInterval.Duration != DefaultInterval {
		t.Errorf("interval = %v, want %v", cfg.ReportInterval.Duration, DefaultInterval)
	}
	if got, want := cfg.StaleAfter(), 3*DefaultInterval; got != want {
		t.Errorf("StaleAfter() = %v, want %v", got, want)
	}
}

func TestLoadMasterFull(t *testing.T) {
	path := write(t, "master.toml", `
listen = "0.0.0.0:9000"
secret = "hunter2"
report_interval = "5s"
`)

	cfg, err := LoadMaster(path)
	if err != nil {
		t.Fatalf("LoadMaster: %v", err)
	}
	if cfg.Listen != "0.0.0.0:9000" {
		t.Errorf("listen = %q", cfg.Listen)
	}
	if got, want := cfg.StaleAfter(), 15*time.Second; got != want {
		t.Errorf("StaleAfter() = %v, want %v", got, want)
	}
}

func TestLoadMasterNeedsSecret(t *testing.T) {
	path := write(t, "master.toml", `listen = "127.0.0.1:8080"`)
	if _, err := LoadMaster(path); err == nil {
		t.Error("expected an error when secret is missing")
	}
}

func TestLoadMasterRejectsUnknownKey(t *testing.T) {
	path := write(t, "master.toml", "secret = \"x\"\nlistne = \"typo\"\n")
	if _, err := LoadMaster(path); err == nil {
		t.Error("expected an error for an unknown key")
	}
}

func TestLoadMasterBadDuration(t *testing.T) {
	path := write(t, "master.toml", "secret = \"x\"\nreport_interval = \"15\"\n")
	if _, err := LoadMaster(path); err == nil {
		t.Error("expected an error for a duration without a unit")
	}
}

func TestLoadNodeDefaults(t *testing.T) {
	path := write(t, "node.toml", `
master = "http://127.0.0.1:8080"
secret = "hunter2"
`)

	cfg, err := LoadNode(path)
	if err != nil {
		t.Fatalf("LoadNode: %v", err)
	}
	if cfg.Interval.Duration != DefaultInterval {
		t.Errorf("interval = %v, want %v", cfg.Interval.Duration, DefaultInterval)
	}
	if cfg.ReportPath != DefaultReportPath {
		t.Errorf("report_path = %q, want %q", cfg.ReportPath, DefaultReportPath)
	}
	want := []string{"systemctl", "reboot"}
	if len(cfg.RebootCommand) != len(want) || cfg.RebootCommand[0] != want[0] {
		t.Errorf("reboot_command = %v, want %v", cfg.RebootCommand, want)
	}
}

func TestLoadNodeFull(t *testing.T) {
	path := write(t, "node.toml", `
master = "http://10.0.0.1:8080"
secret = "hunter2"
interval = "5s"
hostname = "web-1"
report_path = "/var"
reboot_command = ["/sbin/reboot", "-f"]
`)

	cfg, err := LoadNode(path)
	if err != nil {
		t.Fatalf("LoadNode: %v", err)
	}
	if cfg.Hostname != "web-1" || cfg.ReportPath != "/var" {
		t.Errorf("unexpected config: %+v", cfg)
	}
	if got := cfg.RebootCommand; len(got) != 2 || got[1] != "-f" {
		t.Errorf("reboot_command = %v", got)
	}
}

func TestLoadNodeNeedsMasterAndSecret(t *testing.T) {
	cases := map[string]string{
		"no master": `secret = "x"`,
		"no secret": `master = "http://127.0.0.1:8080"`,
	}
	for name, content := range cases {
		if _, err := LoadNode(write(t, "node.toml", content)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := LoadMaster(filepath.Join(t.TempDir(), "nope.toml")); err == nil {
		t.Error("expected an error for a missing file")
	}
}

func TestExampleConfigsLoad(t *testing.T) {
	// The shipped examples must stay valid as fields are added.
	if _, err := LoadMaster("../../examples/master.toml"); err != nil {
		t.Errorf("examples/master.toml: %v", err)
	}
	if _, err := LoadNode("../../examples/node.toml"); err != nil {
		t.Errorf("examples/node.toml: %v", err)
	}
}
