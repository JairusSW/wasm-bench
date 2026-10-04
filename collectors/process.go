// Package collectors exposes OS evidence with explicit observation quality.
package collectors

import (
	"bufio"
	"fmt"
	"github.com/wasmbench/wasmbench/protocol"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

func snapshotProcfs(pid int, phase string) []protocol.Observation {
	values := map[string]float64{}
	status := "unsupported"
	reason := "/proc process collectors require Linux"
	if runtime.GOOS == "linux" {
		status = "unavailable"
		reason = "process exited or procfs unavailable"
		if f, err := os.Open(fmt.Sprintf("/proc/%d/status", pid)); err == nil {
			defer f.Close()
			s := bufio.NewScanner(f)
			for s.Scan() {
				parts := strings.Fields(s.Text())
				if len(parts) >= 2 {
					v, _ := strconv.ParseFloat(parts[1], 64)
					if parts[0] == "VmRSS:" {
						values["process.rss"] = v * 1024
					}
					if parts[0] == "VmSize:" {
						values["process.virtual"] = v * 1024
					}
				}
			}
		}
		if f, err := os.Open(fmt.Sprintf("/proc/%d/smaps_rollup", pid)); err == nil {
			defer f.Close()
			s := bufio.NewScanner(f)
			for s.Scan() {
				parts := strings.Fields(s.Text())
				if len(parts) >= 2 {
					v, _ := strconv.ParseFloat(parts[1], 64)
					switch parts[0] {
					case "Pss:":
						values["process.pss"] = v * 1024
					case "Private_Clean:", "Private_Dirty:":
						values["process.private"] += v * 1024
					}
				}
			}
		} else if os.IsPermission(err) {
			status = "permission_denied"
			reason = err.Error()
		}
	}
	var out []protocol.Observation
	for _, metric := range []string{"process.rss", "process.pss", "process.private", "process.virtual"} {
		o := protocol.Observation{Metric: metric, DefinitionVersion: 1, Unit: "bytes", Scope: "adapter_process", Phase: phase, Collector: "procfs", CollectorVersion: "1", Quality: "boundary_snapshot_only", Profile: "memory", Status: status, Reason: reason, Denominator: "process"}
		if v, ok := values[metric]; ok {
			o.Value = protocol.Value(v)
			o.Status = "available"
			o.Reason = ""
		}
		out = append(out, o)
	}
	return out
}

type Sampler struct {
	done         chan struct{}
	finished     chan struct{}
	once         sync.Once
	observations []protocol.Observation
}

func StartSampler(pid int, phase string, interval time.Duration) *Sampler {
	s := &Sampler{done: make(chan struct{}), finished: make(chan struct{})}
	go func() {
		defer close(s.finished)
		peaks := map[string]protocol.Observation{}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			for _, o := range Snapshot(pid, phase) {
				if o.Value != nil {
					old, ok := peaks[o.Metric]
					if !ok || *o.Value > *old.Value {
						o.Quality = "sampled_observed_peak"
						o.Denominator = "process_during_batch_RPC"
						peaks[o.Metric] = o
					}
				}
			}
			select {
			case <-s.done:
				for _, o := range peaks {
					s.observations = append(s.observations, o)
				}
				return
			case <-ticker.C:
			}
		}
	}()
	return s
}
func (s *Sampler) Stop() []protocol.Observation {
	s.once.Do(func() { close(s.done) })
	<-s.finished
	return s.observations
}
