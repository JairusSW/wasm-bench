package collectors

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/protocol"
)

func TestRetainedSnapshotProcessEvidence(t *testing.T) {
	root := os.Getenv("WASMBENCH_SNAPSHOT_PROC_EVIDENCE_DIR")
	if root == "" {
		t.Skip("requires completed native collector evidence")
	}
	var readings []SnapshotProcessReading
	for _, role := range []string{"source", "template", "restored"} {
		data, err := os.ReadFile(filepath.Join(root, role+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var r SnapshotProcessReading
		if err := json.Unmarshal(data, &r); err != nil {
			t.Fatal(err)
		}
		parent := r.ParentPID
		if len(readings) > 0 {
			parent = readings[len(readings)-1].Process.PID
		}
		v, err := ValidateSnapshotProcessReading(r, r.Process, parent)
		if err != nil {
			t.Fatal(role, err)
		}
		if v.RSSBytes == 0 || v.VirtualBytes == 0 {
			t.Fatal("lost real process footprint")
		}
		for _, previous := range readings {
			if previous.Process.PID == r.Process.PID {
				t.Fatal("aliased live processes")
			}
		}
		readings = append(readings, r)
	}
}

func procStat(pid, parent uint32, birth string) string {
	f := make([]string, 20)
	for i := range f {
		f[i] = "0"
	}
	f[0], f[1], f[19] = "S", fmt.Sprint(parent), birth
	return fmt.Sprintf("%d (comm with ) parentheses\nand spaces) %s\n", pid, strings.Join(f, " "))
}
func procReading() SnapshotProcessReading {
	rollup := "001000-002000 ---p 00000000 00:00 0 [rollup]\nPss: 3 kB\nPrivate_Clean: 1 kB\nPrivate_Dirty: 2 kB\n"
	return SnapshotProcessReading{Version: SnapshotProcessCollectorVersion,
		Process: protocol.SnapshotProcess{PID: 102, StartTimeTicks: "9007199254740993"}, ParentPID: 101,
		StartNS: 0, EndNS: 10, StatBefore: procStat(102, 101, "9007199254740993"), StatAfter: procStat(102, 101, "9007199254740993"),
		Status: "Name:\tcomm\nPid:\t102\nPPid:\t101\nState:\tS (sleeping)\nThreads:\t1\nVmRSS:\t4 kB\nVmSize:\t8 kB\n", SmapsRollup: &rollup, SmapsStatus: "available"}
}
func TestSnapshotProcessRawEvidence(t *testing.T) {
	r := procReading()
	v, err := ValidateSnapshotProcessReading(r, r.Process, r.ParentPID)
	if err != nil {
		t.Fatal(err)
	}
	if v.RSSBytes != 4096 || v.VirtualBytes != 8192 || v.Threads != 1 || v.PSSBytes == nil || *v.PSSBytes != 3072 || v.PrivateBytes == nil || *v.PrivateBytes != 3072 {
		t.Fatal("lost raw footprint", v)
	}
	for _, availability := range []string{"permission_denied", "unavailable"} {
		r.SmapsStatus, r.SmapsReason, r.SmapsRollup = availability, "cannot read smaps", nil
		v, err = ValidateSnapshotProcessReading(r, r.Process, r.ParentPID)
		if err != nil || v.PSSBytes != nil || v.PrivateBytes != nil || v.RSSBytes != 4096 {
			t.Fatal("missing smaps was zero or hid status RSS", v, err)
		}
	}
}
func TestSnapshotProcessRejectsForgedOrChangingEvidence(t *testing.T) {
	for name, change := range map[string]func(*SnapshotProcessReading){
		"version":             func(r *SnapshotProcessReading) { r.Version = "unknown" },
		"negative window":     func(r *SnapshotProcessReading) { r.StartNS = -1 },
		"reversed window":     func(r *SnapshotProcessReading) { r.EndNS = -1 },
		"reused PID":          func(r *SnapshotProcessReading) { r.StatAfter = procStat(102, 101, "9007199254740994") },
		"wrong PID":           func(r *SnapshotProcessReading) { r.StatBefore = procStat(103, 101, r.Process.StartTimeTicks) },
		"reparented":          func(r *SnapshotProcessReading) { r.StatAfter = procStat(102, 1, r.Process.StartTimeTicks) },
		"zombie":              func(r *SnapshotProcessReading) { r.StatAfter = strings.Replace(r.StatAfter, ") S ", ") Z ", 1) },
		"truncated":           func(r *SnapshotProcessReading) { r.StatAfter = "102 (comm) S 101" },
		"noncanonical birth":  func(r *SnapshotProcessReading) { r.StatAfter = procStat(102, 101, "09007199254740993") },
		"zero birth":          func(r *SnapshotProcessReading) { r.StatAfter = procStat(102, 101, "0") },
		"wrong status parent": func(r *SnapshotProcessReading) { r.Status = strings.Replace(r.Status, "PPid:\t101", "PPid:\t1", 1) },
		"dead status":         func(r *SnapshotProcessReading) { r.Status = strings.Replace(r.Status, "S (sleeping)", "Z (zombie)", 1) },
		"duplicate status":    func(r *SnapshotProcessReading) { r.Status += "VmRSS: 1 kB\n" },
		"RSS invalid":         func(r *SnapshotProcessReading) { r.Status = strings.Replace(r.Status, "4 kB", "bad kB", 1) },
		"RSS negative":        func(r *SnapshotProcessReading) { r.Status = strings.Replace(r.Status, "4 kB", "-4 kB", 1) },
		"RSS wrong units":     func(r *SnapshotProcessReading) { r.Status = strings.Replace(r.Status, "4 kB", "4 bytes", 1) },
		"RSS overflow": func(r *SnapshotProcessReading) {
			r.Status = strings.Replace(r.Status, "4 kB", "18446744073709551615 kB", 1)
		},
		"no threads":      func(r *SnapshotProcessReading) { r.Status = strings.Replace(r.Status, "Threads:\t1", "Threads:\t0", 1) },
		"smaps missing":   func(r *SnapshotProcessReading) { r.SmapsRollup = nil },
		"smaps invalid":   func(r *SnapshotProcessReading) { s := "Pss: bad kB\n"; r.SmapsRollup = &s },
		"smaps duplicate": func(r *SnapshotProcessReading) { s := *r.SmapsRollup + "Pss: 0 kB\n"; r.SmapsRollup = &s },
		"missing clean": func(r *SnapshotProcessReading) {
			s := strings.Replace(*r.SmapsRollup, "Private_Clean: 1 kB\n", "", 1)
			r.SmapsRollup = &s
		},
		"private sum overflow": func(r *SnapshotProcessReading) {
			s := "Pss: 0 kB\nPrivate_Clean: 18014398509481983 kB\nPrivate_Dirty: 18014398509481983 kB\n"
			r.SmapsRollup = &s
		},
		"unavailable with values": func(r *SnapshotProcessReading) { r.SmapsStatus = "unavailable"; r.SmapsReason = "denied" },
		"missing error":           func(r *SnapshotProcessReading) { r.SmapsStatus = "permission_denied"; r.SmapsRollup = nil },
	} {
		t.Run(name, func(t *testing.T) {
			r := procReading()
			change(&r)
			if _, err := ValidateSnapshotProcessReading(r, r.Process, r.ParentPID); err == nil {
				t.Fatal("accepted invalid process reading")
			}
		})
	}
}
func TestSnapshotProcessCollectorBracketsReads(t *testing.T) {
	for _, mode := range []string{"valid", "reuse", "exit", "denied", "initial mismatch"} {
		t.Run(mode, func(t *testing.T) {
			r := procReading()
			stats := 0
			var paths []string
			read := func(path string) ([]byte, error) {
				paths = append(paths, path)
				switch path {
				case "/proc/102/stat":
					stats++
					if stats == 1 && mode == "initial mismatch" {
						return []byte(procStat(102, 1, r.Process.StartTimeTicks)), nil
					}
					if stats == 2 && mode == "reuse" {
						return []byte(procStat(102, 101, "9007199254740994")), nil
					}
					if stats == 2 && mode == "exit" {
						return nil, os.ErrNotExist
					}
					return []byte(r.StatBefore), nil
				case "/proc/102/status":
					return []byte(r.Status), nil
				case "/proc/102/smaps_rollup":
					if mode == "denied" {
						return nil, os.ErrPermission
					}
					return []byte(*r.SmapsRollup), nil
				default:
					t.Fatal("read an undeclared process", path)
					return nil, os.ErrNotExist
				}
			}
			got, err := collectSnapshotProcess(r.Process, r.ParentPID, time.Now(), read)
			if mode == "reuse" || mode == "exit" || mode == "initial mismatch" {
				if err == nil {
					t.Fatal("accepted unstable lineage")
				}
				if mode == "initial mismatch" && len(paths) != 1 {
					t.Fatal("continued reads after identity mismatch")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(paths) != 4 || paths[0] != "/proc/102/stat" || paths[3] != "/proc/102/stat" {
				t.Fatal("unbracketed collection", paths)
			}
			if mode == "denied" && (got.SmapsStatus != "permission_denied" || got.SmapsRollup != nil) {
				t.Fatal("lost denied smaps evidence")
			}
		})
	}
}
