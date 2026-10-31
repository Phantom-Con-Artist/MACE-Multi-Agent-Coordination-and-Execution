package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSettingsFlags(t *testing.T) {
	t.Setenv("MACE_HOME", t.TempDir())
	t.Setenv("MACE_HTTP_PORT", "8080")
	t.Setenv("MACE_HTTP_HOST", "0.0.0.0")
	for _, args := range [][]string{{"--port", "18090", "--host", "127.0.0.1"}, {"--port=18090", "--host=127.0.0.1"}, {"-port", "18090", "-host", "127.0.0.1"}} {
		c, e := loadSettings(args)
		if e != nil || c.Port != 18090 || c.Host != "127.0.0.1" {
			t.Fatal(c, e)
		}
	}
	for _, args := range [][]string{{"--port", "not-a-port"}, {"--port", "-1"}, {"--port", "65536"}, {"--host"}} {
		if _, e := loadSettings(args); e == nil {
			t.Fatal("accepted invalid settings", args)
		}
	}
}
func TestProcessOwnershipRejectsUnrelatedPID(t *testing.T) {
	t.Setenv("MACE_HOME", t.TempDir())
	c, e := loadSettings(nil)
	if e != nil {
		t.Fatal(e)
	}
	if !alive(os.Getpid()) {
		t.Fatal("current process not alive")
	}
	if c.owns(os.Getpid()) {
		t.Fatal("test process must not be treated as daemon")
	}
	if filepath.Dir(c.PIDFile) != c.State {
		t.Fatal("runtime files must share a state directory")
	}
}
func TestServiceUnitCarriesPathAndSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("MACE_HOME", home)
	t.Setenv("MACE_PID_FILE", filepath.Join(home, "maced.pid"))
	c, e := loadSettings([]string{"--port", "18093", "--host", "127.0.0.1"})
	if e != nil {
		t.Fatal(e)
	}
	u := c.unit("/opt/mace bin/mace", "/home/me/.local/bin:/usr/bin")
	for _, want := range []string{
		"Type=forking",
		"PIDFile=" + filepath.Join(home, "maced.pid"),
		`ExecStart="/opt/mace bin/mace" start --port 18093 --host 127.0.0.1`,
		"Environment=PATH=/home/me/.local/bin:/usr/bin",
		"Environment=MACE_HOME=" + home,
		"Restart=on-failure",
		"Environment=MACE_SERVICE=1",
		"WantedBy=default.target",
	} {
		if !strings.Contains(u, want) {
			t.Errorf("unit is missing %q:\n%s", want, u)
		}
	}
}
