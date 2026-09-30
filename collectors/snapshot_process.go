package collectors

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/wasmbench/wasmbench/protocol"
)

const SnapshotProcessCollectorVersion = "linux-snapshot-process-v1"

// SnapshotProcessReading is a non-atomic, bracketed procfs reading of one live
// process. It is not a process-tree peak or a physical COW attribution.
type SnapshotProcessReading struct {
	Version     string                   `json:"version"`
	Process     protocol.SnapshotProcess `json:"process"`
	ParentPID   uint32                   `json:"parent_pid"`
	StartNS     int64                    `json:"start_ns"`
	EndNS       int64                    `json:"end_ns"`
	StatBefore  string                   `json:"stat_before"`
	Status      string                   `json:"status"`
	SmapsRollup *string                  `json:"smaps_rollup,omitempty"`
	SmapsStatus string                   `json:"smaps_status"`
	SmapsReason string                   `json:"smaps_reason,omitempty"`
	StatAfter   string                   `json:"stat_after"`
}

type SnapshotProcessFootprint struct {
	RSSBytes     uint64
	VirtualBytes uint64
	Threads      uint64
	PSSBytes     *uint64
	PrivateBytes *uint64
}

type snapshotStat struct {
	pid, parent uint32
	birth       string
	state       string
}

func canonicalUint(s string, bits int) (uint64, error) {
	n, err := strconv.ParseUint(s, 10, bits)
	if err != nil || strconv.FormatUint(n, 10) != s {
		return 0, fmt.Errorf("invalid canonical unsigned integer %q", s)
	}
	return n, nil
}

func parseSnapshotStat(raw string) (snapshotStat, error) {
	var out snapshotStat
	open, close := strings.Index(raw, " ("), strings.LastIndex(raw, ") ")
	if open < 1 || close <= open {
		return out, fmt.Errorf("invalid proc stat comm framing")
	}
	pid, err := canonicalUint(raw[:open], 32)
	if err != nil || pid == 0 || pid > math.MaxInt32 {
		return out, fmt.Errorf("invalid proc stat PID")
	}
	fields := strings.Fields(raw[close+2:])
	if len(fields) < 20 {
		return out, fmt.Errorf("truncated proc stat")
	}
	parent, err := canonicalUint(fields[1], 32)
	if err != nil || parent > math.MaxInt32 {
		return out, fmt.Errorf("invalid proc stat parent")
	}
	birth, err := canonicalUint(fields[19], 64)
	if err != nil || birth == 0 {
		return out, fmt.Errorf("invalid proc stat birth")
	}
	if !strings.Contains("RSDTtIWPK", fields[0]) || len(fields[0]) != 1 {
		return out, fmt.Errorf("process is dead or has unknown state")
	}
	return snapshotStat{uint32(pid), uint32(parent), fields[19], fields[0]}, nil
}

func procFields(raw string) (map[string][]string, error) {
	out := map[string][]string{}
	for _, line := range strings.Split(raw, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		} // smaps_rollup starts with an address-range line.
		if _, duplicate := out[key]; duplicate {
			return nil, fmt.Errorf("duplicate proc field %s", key)
		}
		out[key] = strings.Fields(value)
	}
	return out, nil
}

func procCount(fields map[string][]string, key string, bytes bool) (uint64, error) {
	f := fields[key]
	if (bytes && (len(f) != 2 || f[1] != "kB")) || (!bytes && len(f) != 1) {
		return 0, fmt.Errorf("missing/malformed proc field %s", key)
	}
	n, err := canonicalUint(f[0], 64)
	if err != nil {
		return 0, err
	}
	if bytes {
		if n > math.MaxUint64/1024 {
			return 0, fmt.Errorf("proc byte count overflow")
		}
		n *= 1024
	}
	return n, nil
}

// ValidateSnapshotProcessReading re-derives identities and footprint from raw
// evidence, rather than trusting decoded numbers retained by a collector.
func ValidateSnapshotProcessReading(r SnapshotProcessReading, want protocol.SnapshotProcess, parent uint32) (SnapshotProcessFootprint, error) {
	var out SnapshotProcessFootprint
	if r.Version != SnapshotProcessCollectorVersion || r.Process != want || r.ParentPID != parent || r.StartNS < 0 || r.EndNS < r.StartNS {
		return out, fmt.Errorf("invalid snapshot process reading envelope")
	}
	for _, raw := range []string{r.StatBefore, r.StatAfter} {
		s, err := parseSnapshotStat(raw)
		if err != nil {
			return out, err
		}
		if s.pid != want.PID || s.birth != want.StartTimeTicks || s.parent != parent {
			return out, fmt.Errorf("process identity/parent changed or differs from expected lineage")
		}
	}
	status, err := procFields(r.Status)
	if err != nil {
		return out, err
	}
	pid, err := procCount(status, "Pid", false)
	if err != nil || pid != uint64(want.PID) {
		return out, fmt.Errorf("status PID differs from live identity")
	}
	ppid, err := procCount(status, "PPid", false)
	if err != nil || ppid != uint64(parent) {
		return out, fmt.Errorf("status parent differs from expected lineage")
	}
	state := status["State"]
	if len(state) < 1 || len(state[0]) != 1 || !strings.Contains("RSDTtIWPK", state[0]) {
		return out, fmt.Errorf("status is not a live process")
	}
	if out.Threads, err = procCount(status, "Threads", false); err != nil || out.Threads == 0 {
		return out, fmt.Errorf("missing live thread count")
	}
	if out.RSSBytes, err = procCount(status, "VmRSS", true); err != nil {
		return out, err
	}
	if out.VirtualBytes, err = procCount(status, "VmSize", true); err != nil {
		return out, err
	}
	switch r.SmapsStatus {
	case "available":
		if r.SmapsRollup == nil || r.SmapsReason != "" {
			return out, fmt.Errorf("available smaps requires raw evidence and no error")
		}
		fields, err := procFields(*r.SmapsRollup)
		if err != nil {
			return out, err
		}
		pss, err := procCount(fields, "Pss", true)
		if err != nil {
			return out, err
		}
		clean, err := procCount(fields, "Private_Clean", true)
		if err != nil {
			return out, err
		}
		dirty, err := procCount(fields, "Private_Dirty", true)
		if err != nil {
			return out, err
		}
		if clean > math.MaxUint64-dirty {
			return out, fmt.Errorf("private byte sum overflow")
		}
		private := clean + dirty
		out.PSSBytes, out.PrivateBytes = &pss, &private
	case "permission_denied", "unavailable":
		if r.SmapsRollup != nil || r.SmapsReason == "" {
			return out, fmt.Errorf("missing smaps must retain a reason, not partial values")
		}
	default:
		return out, fmt.Errorf("invalid smaps availability")
	}
	return out, nil
}

// CollectSnapshotProcess reads only the declared PID, brackets all reads with
// stat, and never discovers, signals, or claims ownership of another process.
func CollectSnapshotProcess(want protocol.SnapshotProcess, parent uint32, origin time.Time) (SnapshotProcessReading, error) {
	if runtime.GOOS != "linux" {
		return SnapshotProcessReading{}, fmt.Errorf("snapshot process evidence requires native Linux procfs")
	}
	return collectSnapshotProcess(want, parent, origin, os.ReadFile)
}

func collectSnapshotProcess(want protocol.SnapshotProcess, parent uint32, origin time.Time, read func(string) ([]byte, error)) (SnapshotProcessReading, error) {
	r := SnapshotProcessReading{Version: SnapshotProcessCollectorVersion, Process: want, ParentPID: parent, StartNS: time.Since(origin).Nanoseconds()}
	if want.PID == 0 || want.PID > math.MaxInt32 {
		return r, fmt.Errorf("invalid requested process")
	}
	path := fmt.Sprintf("/proc/%d/", want.PID)
	before, err := read(path + "stat")
	if err != nil {
		return r, err
	}
	s, err := parseSnapshotStat(string(before))
	if err != nil || s.pid != want.PID || s.birth != want.StartTimeTicks || s.parent != parent {
		return r, fmt.Errorf("requested process does not match initial proc identity")
	}
	r.StatBefore = string(before)
	status, err := read(path + "status")
	if err != nil {
		return r, err
	}
	r.Status = string(status)
	if data, err := read(path + "smaps_rollup"); err == nil {
		raw := string(data)
		r.SmapsRollup = &raw
		r.SmapsStatus = "available"
	} else {
		r.SmapsStatus, r.SmapsReason = "unavailable", err.Error()
		if os.IsPermission(err) {
			r.SmapsStatus = "permission_denied"
		}
	}
	after, err := read(path + "stat")
	if err != nil {
		return r, err
	}
	r.StatAfter, r.EndNS = string(after), time.Since(origin).Nanoseconds()
	_, err = ValidateSnapshotProcessReading(r, want, parent)
	return r, err
}
