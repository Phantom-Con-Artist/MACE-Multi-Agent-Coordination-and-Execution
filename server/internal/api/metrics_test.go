package api

import (
	"context"
	"testing"
)

func TestMachineSample(t *testing.T) {
	var m monitor
	m.sample(context.Background())
	s := m.sample(context.Background())
	if s.MemTotal == 0 || s.MemUsed > s.MemTotal || s.Cores == 0 || s.CPU < 0 || s.CPU > 100 || s.Disk <= 0 || s.Disk > 100 {
		t.Fatalf("implausible sample: %+v", s)
	}
}
