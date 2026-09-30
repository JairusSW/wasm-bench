package agent

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type Host struct {
	OS          string            `json:"os"`
	Arch        string            `json:"arch"`
	Hostname    string            `json:"hostname"`
	CPUs        int               `json:"logical_cpus"`
	PageSize    int               `json:"page_size"`
	Kernel      string            `json:"kernel"`
	CPU         string            `json:"cpu_description"`
	Environment map[string]string `json:"environment"`
	Policy      map[string]string `json:"policy"`
	Fingerprint *HostFingerprint  `json:"fingerprint,omitempty"`
}

func IdentifyHost() Host {
	host, _ := os.Hostname()
	h := Host{OS: runtime.GOOS, Arch: runtime.GOARCH, Hostname: host, CPUs: runtime.NumCPU(), PageSize: os.Getpagesize(), Environment: map[string]string{}, Policy: map[string]string{"cpu_affinity": "uncontrolled", "frequency": "uncontrolled", "numa": "uncontrolled", "smt": "uncontrolled", "filesystem_cache": "uncontrolled", "resource_budget": "uncontrolled", "huge_pages": "uncontrolled"}}
	if b, e := exec.Command("uname", "-srv").Output(); e == nil {
		h.Kernel = strings.TrimSpace(string(b))
	}
	if runtime.GOOS == "darwin" {
		if b, e := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output(); e == nil {
			h.CPU = strings.TrimSpace(string(b))
		}
	} else if runtime.GOOS == "linux" {
		h.Fingerprint, h.CPU = linuxHostFacts(os.DirFS("/"))
	}
	for _, key := range []string{"GOMAXPROCS", "GOGC", "GOMEMLIMIT", "NODE_OPTIONS"} {
		if v, ok := os.LookupEnv(key); ok {
			h.Environment[key] = v
		}
	}
	return h
}
