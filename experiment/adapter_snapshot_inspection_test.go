package experiment_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

type snapshotLiveRecord struct {
	Boundary protocol.SnapshotBoundary           `json:"boundary"`
	Readings []collectors.SnapshotProcessReading `json:"readings"`
}

func TestRetainedNativeSnapshotInspection(t *testing.T) {
	root := os.Getenv("WASMBENCH_SNAPSHOT_RETAINED_LIVE_EVIDENCE_DIR")
	if root == "" {
		t.Skip("requires completed native live inspection evidence")
	}
	workloads, err := corpus.Generate(t.TempDir(), "process-snapshots")
	if err != nil {
		t.Fatal(err)
	}
	for _, backend := range []string{"cranelift", "winch"} {
		data, err := os.ReadFile(filepath.Join(root, backend+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var record struct {
			Version     string                       `json:"version"`
			Diagnostics protocol.SnapshotDiagnostics `json:"diagnostics"`
			Records     []snapshotLiveRecord         `json:"records"`
		}
		if err := json.Unmarshal(data, &record); err != nil {
			t.Fatal(err)
		}
		if record.Version != "native-snapshot-live-inspection-test-v1" || len(record.Records) != 14 {
			t.Fatal("unexpected native inspection scope/coverage")
		}
		var events []protocol.SnapshotBoundary
		var controller uint32
		var previousEnd int64
		for _, frame := range record.Records {
			if len(frame.Readings) < 2 {
				t.Fatal("missing source/template reading")
			}
			b := frame.Boundary
			events = append(events, b)
			if controller == 0 {
				controller = frame.Readings[0].ParentPID
			}
			items := []struct {
				ref    protocol.SnapshotProcess
				parent uint32
			}{{b.Source, controller}, {b.Template, b.Source.PID}}
			if b.Restored != nil {
				items = append(items, struct {
					ref    protocol.SnapshotProcess
					parent uint32
				}{*b.Restored, b.Template.PID})
			}
			if len(frame.Readings) != len(items) {
				t.Fatal("missing/extra live process reading")
			}
			for i, item := range items {
				r := frame.Readings[i]
				footprint, err := collectors.ValidateSnapshotProcessReading(r, item.ref, item.parent)
				if err != nil {
					t.Fatal(err)
				}
				if footprint.Threads != 1 || r.StartNS < previousEnd {
					t.Fatal("invalid process threads or collector order")
				}
				previousEnd = r.EndNS
			}
		}
		r := protocol.RunRequest{Scenario: "process-snapshot-execute", Samples: 2, Operations: 1, PhaseBarriers: true}
		if err := protocol.VerifySnapshotInspection(workloads[0], r, events, record.Diagnostics); err != nil {
			t.Fatal(err)
		}
		e := experiment.SnapshotMemoryEvidence{Version: experiment.SnapshotMemoryVersion, ControllerPID: controller, Diagnostics: record.Diagnostics}
		for _, frame := range record.Records {
			e.Records = append(e.Records, experiment.SnapshotMemoryRecord{Boundary: frame.Boundary, Readings: frame.Readings})
		}
		if err := experiment.ValidateSnapshotMemoryEvidence(workloads[0], r, e); err != nil {
			t.Fatal(err)
		}
	}
}

func TestNativeSnapshotLiveInspection(t *testing.T) {
	binary := os.Getenv("WASMBENCH_SNAPSHOT_LIVE_ADAPTER")
	if binary == "" || runtime.GOOS != "linux" {
		t.Skip("requires built native Linux snapshot adapter")
	}
	for _, backend := range []string{"cranelift", "winch"} {
		t.Run(backend, func(t *testing.T) {
			root := t.TempDir()
			workloads, err := corpus.Generate(root, "process-snapshots")
			if err != nil {
				t.Fatal(err)
			}
			w := workloads[0]
			c, err := agent.Start(context.Background(), []string{binary, "--adapter=" + backend}, filepath.Join(root, "adapter.log"), 30*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			described, err := c.Call(protocol.Request{Method: "describe"})
			if err != nil {
				t.Fatal(err)
			}
			if !described.Description.Capabilities["can_inspect_linux_snapshot_lineage"] {
				t.Fatal("missing inspection capability")
			}
			prep := protocol.Preparation{Artifact: w.Artifact, ArtifactSHA256: w.SHA256, Workload: w, Profile: "memory"}
			if _, err := c.Call(protocol.Request{Method: "prepare", Prepare: &prep}); err != nil {
				t.Fatal(err)
			}
			r := protocol.RunRequest{Scenario: "process-snapshot-execute", Samples: 2, Operations: 1, PhaseBarriers: true}
			if err := protocol.ValidateSnapshotInspection(prep, r); err != nil {
				t.Fatal(err)
			}
			var events []protocol.SnapshotBoundary
			var records []snapshotLiveRecord
			origin := time.Now()
			response, err := c.CallSnapshotInspection(protocol.Request{Method: "inspect", Run: &r}, func(b protocol.SnapshotBoundary) error {
				if b.Source.PID != uint32(c.PID()) {
					return fmt.Errorf("source differs from launched adapter")
				}
				record := snapshotLiveRecord{Boundary: b}
				items := []struct {
					ref    protocol.SnapshotProcess
					parent uint32
				}{{b.Source, uint32(os.Getpid())}, {b.Template, b.Source.PID}}
				if b.Restored != nil {
					items = append(items, struct {
						ref    protocol.SnapshotProcess
						parent uint32
					}{*b.Restored, b.Template.PID})
				}
				for _, item := range items {
					reading, err := collectors.CollectSnapshotProcess(item.ref, item.parent, origin)
					if err != nil {
						return err
					}
					footprint, err := collectors.ValidateSnapshotProcessReading(reading, item.ref, item.parent)
					if err != nil {
						return err
					}
					if footprint.Threads != 1 {
						return fmt.Errorf("snapshot process is not single-threaded")
					}
					record.Readings = append(record.Readings, reading)
				}
				events = append(events, b)
				records = append(records, record)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if response.SnapshotDiagnostics == nil || len(response.Samples) != 0 {
				t.Fatal("missing diagnostics or promoted headline samples")
			}
			if err := protocol.VerifySnapshotInspection(w, r, events, *response.SnapshotDiagnostics); err != nil {
				t.Fatal(err)
			}
			if len(records) != 14 {
				t.Fatal("missing live boundaries")
			}
			if output := os.Getenv("WASMBENCH_SNAPSHOT_LIVE_EVIDENCE_DIR"); output != "" {
				f, err := os.OpenFile(filepath.Join(output, backend+".json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
				if err != nil {
					t.Fatal(err)
				}
				err = json.NewEncoder(f).Encode(struct {
					Version     string                        `json:"version"`
					Diagnostics *protocol.SnapshotDiagnostics `json:"diagnostics"`
					Records     []snapshotLiveRecord          `json:"records"`
				}{"native-snapshot-live-inspection-test-v1", response.SnapshotDiagnostics, records})
				closeErr := f.Close()
				if err != nil {
					t.Fatal(err)
				}
				if closeErr != nil {
					t.Fatal(closeErr)
				}
			}
			if _, err := c.Call(protocol.Request{Method: "close"}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
