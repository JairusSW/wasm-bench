package collectors

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/wasmbench/wasmbench/protocol"
)

// Snapshot records current resident and virtual sizes on both Darwin architectures.
// ps reports KiB; these observations use bytes, like the Linux collector.
func Snapshot(pid int, phase string) []protocol.Observation {
	values := map[string]float64{}
	reason := "invalid process ID"
	if pid > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		output, err := exec.CommandContext(ctx, "/bin/ps", "-o", "pid=,rss=,vsz=", "-p", strconv.Itoa(pid)).Output()
		if err == nil {
			values, err = parseDarwinProcessSizes(string(output), pid)
		}
		reason = ""
		if err != nil {
			reason = "process exited or ps unavailable: " + err.Error()
		}
	}
	var out []protocol.Observation
	for _, metric := range []string{"process.rss", "process.pss", "process.private", "process.virtual"} {
		o := protocol.Observation{Metric: metric, DefinitionVersion: 1, Unit: "bytes", Scope: "adapter_process", Phase: phase, Collector: "darwin_ps", CollectorVersion: "1", Quality: "boundary_snapshot_only", Profile: "memory", Status: "unavailable", Reason: reason, Denominator: "process"}
		if metric == "process.pss" || metric == "process.private" {
			o.Status = "unsupported"
			o.Reason = "Darwin ps does not expose this process memory metric"
		} else if v, ok := values[metric]; ok {
			o.Status = "available"
			o.Value = protocol.Value(v)
			o.Reason = ""
		}
		out = append(out, o)
	}
	return out
}

func parseDarwinProcessSizes(output string, pid int) (map[string]float64, error) {
	fields := strings.Fields(output)
	if len(fields) != 3 || fields[0] != strconv.Itoa(pid) {
		return nil, fmt.Errorf("unexpected ps output for PID %d", pid)
	}
	values := make(map[string]float64, 2)
	for i, metric := range []string{"process.rss", "process.virtual"} {
		kib, err := strconv.ParseUint(fields[i+1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid %s from ps: %w", metric, err)
		}
		values[metric] = float64(kib) * 1024
	}
	return values, nil
}
