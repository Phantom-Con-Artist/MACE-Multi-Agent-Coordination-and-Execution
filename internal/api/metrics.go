package api

import (
	"bufio"
	"context"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Sample is one reading of this host's load. GPU fields are nil when no
// supported GPU reports utilization.
type Sample struct {
	At       time.Time `json:"at"`
	CPU      float64   `json:"cpu"`       // percent busy across all cores
	MemUsed  uint64    `json:"mem_used"`  // bytes
	MemTotal uint64    `json:"mem_total"` // bytes
	Disk     float64   `json:"disk"`      // percent of the root filesystem used
	Load     float64   `json:"load"`      // 1 minute load average
	Cores    int       `json:"cores"`
	GPU      *float64  `json:"gpu,omitempty"`      // percent busy
	GPUMem   *float64  `json:"gpu_mem,omitempty"`  // percent of memory used
	GPUName  string    `json:"gpu_name,omitempty"` //
}

const historyLen = 120 // ten minutes at one sample every five seconds

type monitor struct {
	mu        sync.Mutex
	history   []Sample
	idle      uint64
	total     uint64
	gpuProbed bool
	nvidia    bool
}

func cpuTimes() (idle, total uint64, ok bool) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()
	line, _ := bufio.NewReader(f).ReadString('\n')
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0, false
	}
	for i, v := range fields[1:] {
		n, _ := strconv.ParseUint(v, 10, 64)
		total += n
		if i == 3 || i == 4 { // idle and iowait
			idle += n
		}
	}
	return idle, total, true
}

func memInfo() (used, total uint64) {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	var available uint64
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		n, _ := strconv.ParseUint(f[1], 10, 64)
		switch f[0] {
		case "MemTotal:":
			total = n * 1024
		case "MemAvailable:":
			available = n * 1024
		}
	}
	if total > available {
		used = total - available
	}
	return used, total
}

// gpu reads NVIDIA GPUs through nvidia-smi and AMD GPUs through sysfs.
func (m *monitor) gpu(ctx context.Context) (busy, mem *float64, name string) {
	if !m.gpuProbed {
		_, err := exec.LookPath("nvidia-smi")
		m.nvidia, m.gpuProbed = err == nil, true
	}
	if m.nvidia {
		c, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		out, err := exec.CommandContext(c, "nvidia-smi", "--query-gpu=utilization.gpu,memory.used,memory.total,name", "--format=csv,noheader,nounits").Output()
		if err == nil {
			f := strings.Split(strings.SplitN(string(out), "\n", 2)[0], ",")
			if len(f) >= 4 {
				u, _ := strconv.ParseFloat(strings.TrimSpace(f[0]), 64)
				used, _ := strconv.ParseFloat(strings.TrimSpace(f[1]), 64)
				total, _ := strconv.ParseFloat(strings.TrimSpace(f[2]), 64)
				pct := 0.0
				if total > 0 {
					pct = used / total * 100
				}
				return &u, &pct, strings.TrimSpace(f[3])
			}
		}
	}
	for i := 0; i < 4; i++ {
		base := "/sys/class/drm/card" + strconv.Itoa(i) + "/device/"
		b, err := os.ReadFile(base + "gpu_busy_percent")
		if err != nil {
			continue
		}
		u, _ := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
		var pct *float64
		used, e1 := os.ReadFile(base + "mem_info_vram_used")
		total, e2 := os.ReadFile(base + "mem_info_vram_total")
		if e1 == nil && e2 == nil {
			a, _ := strconv.ParseFloat(strings.TrimSpace(string(used)), 64)
			t, _ := strconv.ParseFloat(strings.TrimSpace(string(total)), 64)
			if t > 0 {
				p := a / t * 100
				pct = &p
			}
		}
		return &u, pct, "GPU " + strconv.Itoa(i)
	}
	return nil, nil, ""
}

func (m *monitor) sample(ctx context.Context) Sample {
	s := Sample{At: time.Now().UTC()}
	if idle, total, ok := cpuTimes(); ok {
		m.mu.Lock()
		if m.total > 0 && total > m.total {
			s.CPU = 100 * (1 - float64(idle-m.idle)/float64(total-m.total))
		}
		m.idle, m.total = idle, total
		m.mu.Unlock()
	}
	s.MemUsed, s.MemTotal = memInfo()
	var st syscall.Statfs_t
	if syscall.Statfs("/", &st) == nil && st.Blocks > 0 {
		s.Disk = 100 * float64(st.Blocks-st.Bfree) / float64(st.Blocks)
	}
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		s.Load, _ = strconv.ParseFloat(strings.Fields(string(b))[0], 64)
	}
	s.Cores = runtime.NumCPU()
	s.GPU, s.GPUMem, s.GPUName = m.gpu(ctx)
	return s
}

// watchMachine samples every five seconds, keeps ten minutes of history, and
// streams each sample to connected browsers.
func (h *APIHandler) watchMachine(ctx context.Context) {
	h.machine.sample(ctx) // primes CPU counters
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s := h.machine.sample(ctx)
			h.machine.mu.Lock()
			h.machine.history = append(h.machine.history, s)
			if len(h.machine.history) > historyLen {
				h.machine.history = h.machine.history[len(h.machine.history)-historyLen:]
			}
			h.machine.mu.Unlock()
			h.events.publish("metrics", map[string]any{"host": "local", "sample": s})
		}
	}
}

// metrics serves GET /api/v1/metrics: this host's recent samples.
func (h *APIHandler) metrics(w http.ResponseWriter, r *http.Request) {
	h.machine.mu.Lock()
	history := append([]Sample{}, h.machine.history...)
	h.machine.mu.Unlock()
	host, _ := os.Hostname()
	writeJSON(w, 200, map[string]any{"host": host, "history": history})
}

// peerMetrics serves GET /api/v1/peers/{id}/metrics from a connected host.
func (h *APIHandler) peerMetrics(w http.ResponseWriter, r *http.Request) {
	var p Peer
	if h.peerDoc(r.Context(), routeParam(r, "id"), &p) != nil {
		http.Error(w, "That host is not connected.", 404)
		return
	}
	var out map[string]any
	if e := h.peerRequest(r.Context(), p, "/metrics", nil, &out); e != nil {
		http.Error(w, e.Error(), 502)
		return
	}
	writeJSON(w, 200, out)
}
