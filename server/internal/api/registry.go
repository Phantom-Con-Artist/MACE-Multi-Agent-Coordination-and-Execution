package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// The resource registry: what each machine actually has, measured by MACE on
// that machine and never taken from what an agent says. Linked machines
// receive it with the node ping. The lead staffs jobs by it, and the hub
// refuses a claim from a machine that lacks what a job needs. See
// docs/DESIGN-COMPANY.md.

// MachineFacts is one machine's measured resources.
type MachineFacts struct {
	Node     string    `json:"node"`
	Name     string    `json:"name"`
	OS       string    `json:"os"`
	OSName   string    `json:"os_name"`
	Version  string    `json:"version"`
	Kernel   string    `json:"kernel"`
	Arch     string    `json:"arch"`
	Cores    int       `json:"cores"`
	MemoryGB int       `json:"memory_gb"`
	GPUs     []GPU     `json:"gpus,omitempty"`
	Tools    []string  `json:"tools"`
	DiskGB   int       `json:"disk_gb"`
	Battery  *Battery  `json:"battery,omitempty"`
	Measured time.Time `json:"measured"`
}

// GPU is one graphics card, with its compute stack when one is installed.
type GPU struct {
	Vendor   string `json:"vendor"`
	Model    string `json:"model"`
	MemoryGB int    `json:"memory_gb"`
	CUDA     string `json:"cuda,omitempty"`
}

// registryTools are the CLIs and toolchains the registry looks for on PATH.
var registryTools = []string{"go", "node", "npm", "python3", "rustc", "cargo", "gcc", "clang", "make", "cmake", "docker", "podman", "git", "pacman", "dnf", "apt", "flatpak", "java", "nvcc"}

const factsEvery = 10 * time.Minute

type factsCache struct {
	mu   sync.Mutex
	last MachineFacts
}

// localFacts returns this machine's facts, measuring again when they are old.
func (h *APIHandler) localFacts() MachineFacts {
	h.facts.mu.Lock()
	defer h.facts.mu.Unlock()
	if time.Since(h.facts.last.Measured) < factsEvery {
		h.facts.last.Battery = readBattery() // cheap, and it changes fast
		return h.facts.last
	}
	f := measureFacts(h.fileRoot)
	f.Node = h.selfID()
	f.Name = h.selfNode().Name
	h.facts.last = f
	if f.Node != "" {
		h.store.PutDocument(context.Background(), "machine-facts", f.Node, f)
	}
	return f
}

func measureFacts(root string) MachineFacts {
	f := MachineFacts{Arch: runtime.GOARCH, Cores: runtime.NumCPU(), Measured: time.Now().UTC(), Tools: []string{}}
	if f.Arch == "amd64" {
		f.Arch = "x86_64"
	}
	if b, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			v = strings.Trim(v, `"`)
			switch k {
			case "ID":
				f.OS = v
			case "NAME":
				f.OSName = v
			case "VERSION_ID":
				f.Version = v
			}
		}
	}
	if f.OS == "" {
		f.OS = runtime.GOOS
	}
	if b, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		f.Kernel = strings.TrimSpace(string(b))
	}
	if file, err := os.Open("/proc/meminfo"); err == nil {
		sc := bufio.NewScanner(file)
		for sc.Scan() {
			if fields := strings.Fields(sc.Text()); len(fields) >= 2 && fields[0] == "MemTotal:" {
				kb, _ := strconv.Atoi(fields[1])
				f.MemoryGB = (kb + 512*1024) / (1024 * 1024)
			}
		}
		file.Close()
	}
	var st syscall.Statfs_t
	if root == "" {
		root = "/"
	}
	if syscall.Statfs(root, &st) == nil {
		f.DiskGB = int(st.Bavail * uint64(st.Bsize) >> 30)
	}
	for _, t := range registryTools {
		if _, err := exec.LookPath(t); err == nil {
			f.Tools = append(f.Tools, t)
		}
	}
	f.GPUs = nvidiaGPUs()
	f.Battery = readBattery()
	return f
}

// nvidiaGPUs asks nvidia-smi, when present, for each card and the CUDA version.
func nvidiaGPUs() []GPU {
	if _, err := exec.LookPath("nvidia-smi"); err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "nvidia-smi", "--query-gpu=name,memory.total", "--format=csv,noheader,nounits").Output()
	if err != nil {
		return nil
	}
	cuda := ""
	if head, err := exec.CommandContext(ctx, "nvidia-smi").Output(); err == nil {
		if i := strings.Index(string(head), "CUDA Version:"); i >= 0 {
			if fields := strings.Fields(string(head)[i+len("CUDA Version:"):]); len(fields) > 0 {
				cuda = fields[0]
			}
		}
	}
	gpus := []GPU{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name, mem, ok := strings.Cut(line, ",")
		if !ok {
			continue
		}
		mb, _ := strconv.Atoi(strings.TrimSpace(mem))
		gpus = append(gpus, GPU{Vendor: "nvidia", Model: strings.TrimSpace(name), MemoryGB: (mb + 512) / 1024, CUDA: cuda})
	}
	return gpus
}

// registry returns every known machine's facts, this one's freshly measured.
func (h *APIHandler) registry(ctx context.Context) []MachineFacts {
	self := h.localFacts()
	out := []MachineFacts{self}
	raw, _ := h.store.Documents(ctx, "machine-facts")
	for _, b := range raw {
		var f MachineFacts
		if json.Unmarshal(b, &f) == nil && f.Node != "" && f.Node != self.Node {
			out = append(out, f)
		}
	}
	sort.SliceStable(out[1:], func(i, j int) bool { return out[1+i].Name < out[1+j].Name })
	return out
}

// factsOf returns the facts of the machine a member runs on.
func (h *APIHandler) factsOf(ctx context.Context, member string) (MachineFacts, bool) {
	node, _, remote := strings.Cut(h.localize(member), "::")
	if !remote {
		return h.localFacts(), true
	}
	var f MachineFacts
	return f, h.store.Document(ctx, "machine-facts", node, &f) == nil
}

// summary is one machine's line for agents, with its busy GPUs.
func (f MachineFacts) summary(busy map[string]string) string {
	parts := []string{f.Arch, fmt.Sprintf("%d cores", f.Cores), fmt.Sprintf("%d GB", f.MemoryGB)}
	if len(f.GPUs) == 0 {
		parts = append(parts, "no GPU")
	}
	for _, g := range f.GPUs {
		s := g.Model
		if g.CUDA != "" {
			s += " CUDA " + g.CUDA
		}
		if job := busy[f.Node]; job != "" {
			s += " busy: " + job
		}
		parts = append(parts, s)
	}
	if f.Battery != nil && !f.Battery.Charging {
		parts = append(parts, fmt.Sprintf("on battery %d%%", f.Battery.Percent))
	}
	parts = append(parts, strings.Join(f.Tools, " "))
	system := f.OS
	if f.Version != "" {
		system += " " + f.Version
	}
	return fmt.Sprintf("%s (%s, %s)", f.Name, system, strings.Join(parts, ", "))
}

// machinesLine is the registry as agents read it each turn.
func (h *APIHandler) machinesLine(ctx context.Context) string {
	busy := h.gpuReservations(ctx)
	lines := []string{}
	for _, f := range h.registry(ctx) {
		if f.Name != "" {
			lines = append(lines, f.summary(busy))
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return "MACHINES (measured by MACE): " + strings.Join(lines, " · ") + "\n"
}

// registryView serves GET /api/v1/registry to the owner's apps.
func (h *APIHandler) registryView(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"machines": h.registry(r.Context()), "busy": h.gpuReservations(r.Context())})
}

// needsMet lists the needs a machine does not meet. A need is a tool name,
// an architecture, an OS ID, "gpu", "cuda", "cores>=N", "ram>=N", or
// "on:<machine>" for work that has to run where something lives.
func needsMet(f MachineFacts, needs []string) []string {
	missing := []string{}
	have := map[string]bool{f.OS: true, f.Arch: true, strings.ToLower(f.Name): true}
	for _, t := range f.Tools {
		have[t] = true
	}
	if len(f.GPUs) > 0 {
		have["gpu"] = true
		for _, g := range f.GPUs {
			have[g.Vendor] = true
			if g.CUDA != "" {
				have["cuda"] = true
			}
		}
	}
	for _, n := range needs {
		n = strings.ToLower(strings.TrimSpace(n))
		switch {
		case n == "":
		case strings.HasPrefix(n, "on:"):
			if strings.TrimPrefix(n, "on:") != strings.ToLower(f.Name) {
				missing = append(missing, n)
			}
		case strings.HasPrefix(n, "cores>="):
			if v, _ := strconv.Atoi(strings.TrimPrefix(n, "cores>=")); f.Cores < v {
				missing = append(missing, n)
			}
		case strings.HasPrefix(n, "ram>="):
			if v, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(n, "ram>="), "gb")); f.MemoryGB < v {
				missing = append(missing, n)
			}
		case n == "arm64" || n == "aarch64":
			if f.Arch != "arm64" {
				missing = append(missing, n)
			}
		case n == "x86_64" || n == "amd64":
			if f.Arch != "x86_64" {
				missing = append(missing, n)
			}
		case !have[n]:
			missing = append(missing, n)
		}
	}
	return missing
}
