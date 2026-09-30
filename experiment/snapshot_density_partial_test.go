package experiment_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

type partialDensityEvidence struct {
	Version                  string                           `json:"version"`
	Backend                  string                           `json:"backend"`
	Instances                int                              `json:"instances"`
	FailAfter                int                              `json:"fail_after"`
	ControllerPID            uint32                           `json:"controller_pid"`
	Baseline                 experiment.SnapshotDensityRecord `json:"baseline"`
	Partial                  experiment.SnapshotDensityRecord `json:"partial"`
	AllDeclaredProcessesGone bool                             `json:"all_declared_processes_gone"`
	SourceExitCode           int                              `json:"source_exit_code"`
}

func validatePartialDensity(e partialDensityEvidence) error {
	if e.Version != "native-snapshot-density-partial-cleanup-v1" || (e.Backend != "cranelift" && e.Backend != "winch") || e.FailAfter < 1 || e.FailAfter >= e.Instances || !e.AllDeclaredProcessesGone || e.SourceExitCode != 1 {
		return fmt.Errorf("invalid partial cleanup envelope")
	}
	base := experiment.SnapshotDensityEvidence{Version: experiment.SnapshotDensityEvidenceVersion, ControllerPID: e.ControllerPID, Backend: e.Backend, Instances: e.Instances, Records: []experiment.SnapshotDensityRecord{e.Baseline}}
	if err := experiment.ValidateSnapshotDensityPrefix(base); err != nil {
		return err
	}
	b := e.Partial.Boundary
	if b.Version != "linux-process-snapshot-density-partial-v1" || b.Stage != "partial_provisioning" || b.Instances != e.Instances || len(b.Restored) != e.FailAfter || b.Source != e.Baseline.Boundary.Source || b.Template != e.Baseline.Boundary.Template {
		return fmt.Errorf("partial membership differs from injection")
	}
	// Validate identity shape without passing this incomplete group off as the
	// requested complete group. The retained original boundary remains partial.
	shape := b
	shape.Version = protocol.SnapshotDensityBoundaryVersion
	shape.Stage = "idle"
	shape.Instances = e.FailAfter
	if err := protocol.ValidateSnapshotDensityBoundary(shape); err != nil {
		return err
	}
	refs := append([]protocol.SnapshotProcess{b.Source, b.Template}, b.Restored...)
	if len(e.Partial.Readings) != len(refs) {
		return fmt.Errorf("missing partial process readings")
	}
	end := e.Baseline.Readings[len(e.Baseline.Readings)-1].EndNS
	for i, ref := range refs {
		parent := b.Template.PID
		if i == 0 {
			parent = e.ControllerPID
		} else if i == 1 {
			parent = b.Source.PID
		}
		f, err := collectors.ValidateSnapshotProcessReading(e.Partial.Readings[i], ref, parent)
		if err != nil {
			return err
		}
		if f.Threads != 1 || ref.PID == e.ControllerPID || e.Partial.Readings[i].StartNS < end {
			return fmt.Errorf("invalid partial ownership/clocks")
		}
		end = e.Partial.Readings[i].EndNS
	}
	base.Records = append(base.Records, e.Partial)
	if experiment.ValidateSnapshotDensityEvidence(base) == nil || experiment.ValidateSnapshotDensityPrefix(base) == nil {
		return fmt.Errorf("incomplete group promoted to product evidence")
	}
	return nil
}

func TestNativeSnapshotDensityPartialCleanup(t *testing.T) {
	if runtime.GOOS != "linux" || os.Getenv("WASMBENCH_SNAPSHOT_DENSITY_ADAPTER") == "" {
		t.Skip("requires built native Linux qualifier")
	}
	for _, backend := range []string{"cranelift", "winch"} {
		for _, pair := range [][2]int{{8, 1}, {8, 4}, {8, 7}, {32, 31}} {
			count, failAfter := pair[0], pair[1]
			t.Run(fmt.Sprintf("%s/%d/%d", backend, count, failAfter), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, os.Getenv("WASMBENCH_SNAPSHOT_DENSITY_ADAPTER"), "--density-failure-worker", backend, fmt.Sprint(count), fmt.Sprint(failAfter))
				input, err := cmd.StdinPipe()
				if err != nil {
					t.Fatal(err)
				}
				output, err := cmd.StdoutPipe()
				if err != nil {
					t.Fatal(err)
				}
				var stderr bytes.Buffer
				cmd.Stderr = &stderr
				if err := cmd.Start(); err != nil {
					t.Fatal(err)
				}
				waited := false
				defer func() {
					input.Close()
					if !waited {
						cmd.Process.Kill()
						cmd.Wait()
					}
				}()
				e := partialDensityEvidence{Version: "native-snapshot-density-partial-cleanup-v1", Backend: backend, Instances: count, FailAfter: failAfter, ControllerPID: uint32(os.Getpid())}
				decoder := json.NewDecoder(output)
				origin := time.Now()
				var refs []protocol.SnapshotProcess
				for _, record := range []*experiment.SnapshotDensityRecord{&e.Baseline, &e.Partial} {
					if err := decoder.Decode(&record.Boundary); err != nil {
						t.Fatalf("boundary: %v; %s", err, stderr.String())
					}
					b := record.Boundary
					if b.Source.PID != uint32(cmd.Process.Pid) {
						t.Fatal("not owned source")
					}
					refs = append([]protocol.SnapshotProcess{b.Source, b.Template}, b.Restored...)
					for i, ref := range refs {
						parent := b.Template.PID
						if i == 0 {
							parent = e.ControllerPID
						} else if i == 1 {
							parent = b.Source.PID
						}
						reading, err := collectors.CollectSnapshotProcess(ref, parent, origin)
						if err != nil {
							t.Fatal(err)
						}
						record.Readings = append(record.Readings, reading)
					}
					if _, err := fmt.Fprintln(input, "continue"); err != nil {
						t.Fatal(err)
					}
				}
				input.Close()
				var unexpected json.RawMessage
				if err := decoder.Decode(&unexpected); err != io.EOF {
					t.Fatalf("partial failure emitted success: %s %v", unexpected, err)
				}
				if err := cmd.Wait(); err == nil {
					t.Fatal("injected failure succeeded")
				}
				waited = true
				if ctx.Err() != nil || !cmd.ProcessState.Exited() || cmd.ProcessState.ExitCode() != 1 {
					t.Fatal("cleanup required source signal/timeout", stderr.String())
				}
				e.SourceExitCode = cmd.ProcessState.ExitCode()
				densityProcessesGone(t, refs)
				e.AllDeclaredProcessesGone = true
				if err := validatePartialDensity(e); err != nil {
					t.Fatal(err)
				}
				saveDensityEvidence(t, fmt.Sprintf("partial-%s-%d-%d", backend, count, failAfter), e)
			})
		}
	}
}

func TestRetainedSnapshotDensityPartialCleanup(t *testing.T) {
	root := os.Getenv("WASMBENCH_SNAPSHOT_RETAINED_DENSITY_EVIDENCE_DIR")
	if root == "" {
		t.Skip("requires retained partial-failure qualification")
	}
	for _, backend := range []string{"cranelift", "winch"} {
		for _, pair := range [][2]int{{8, 1}, {8, 4}, {8, 7}, {32, 31}} {
			data, err := os.ReadFile(filepath.Join(root, fmt.Sprintf("partial-%s-%d-%d.json", backend, pair[0], pair[1])))
			if err != nil {
				t.Fatal(err)
			}
			var e partialDensityEvidence
			if err := json.Unmarshal(data, &e); err != nil {
				t.Fatal(err)
			}
			if e.Backend != backend || e.Instances != pair[0] || e.FailAfter != pair[1] {
				t.Fatal("retained injection identity changed")
			}
			if err := validatePartialDensity(e); err != nil {
				t.Fatal(err)
			}
			for name, mutate := range map[string]func(*partialDensityEvidence){
				"missing cleanup":          func(v *partialDensityEvidence) { v.AllDeclaredProcessesGone = false },
				"source was killed":        func(v *partialDensityEvidence) { v.SourceExitCode = -1 },
				"injection mismatch":       func(v *partialDensityEvidence) { v.FailAfter++ },
				"missing child reading":    func(v *partialDensityEvidence) { v.Partial.Readings = v.Partial.Readings[:len(v.Partial.Readings)-1] },
				"duplicate child identity": func(v *partialDensityEvidence) { v.Partial.Boundary.Restored[0] = v.Partial.Boundary.Template },
				"pretend completed boundary": func(v *partialDensityEvidence) {
					v.Partial.Boundary.Version = protocol.SnapshotDensityBoundaryVersion
					v.Partial.Boundary.Stage = "idle"
				},
				"false source identity":   func(v *partialDensityEvidence) { v.Partial.Boundary.Source.PID++ },
				"collector clock overlap": func(v *partialDensityEvidence) { v.Partial.Readings[0].StartNS = 0 },
			} {
				var forged partialDensityEvidence
				if err := json.Unmarshal(data, &forged); err != nil {
					t.Fatal(err)
				}
				mutate(&forged)
				if validatePartialDensity(forged) == nil {
					t.Fatalf("accepted %s", name)
				}
			}
		}
	}
}
