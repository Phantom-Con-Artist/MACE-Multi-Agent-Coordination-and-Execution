package main

import (
	"os"
	"path/filepath"
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
