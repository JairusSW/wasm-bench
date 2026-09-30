package experiment_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func densityNative(t *testing.T, backend string, count int, failAt string) (experiment.SnapshotDensityEvidence, []protocol.SnapshotProcess) {
	t.Helper()
	binary := os.Getenv("WASMBENCH_SNAPSHOT_DENSITY_ADAPTER")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "--density-worker", backend, fmt.Sprint(count))
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
	decoder := json.NewDecoder(output)
	e := experiment.SnapshotDensityEvidence{Version: experiment.SnapshotDensityEvidenceVersion, ControllerPID: uint32(os.Getpid()), Backend: backend, Instances: count}
	origin := time.Now()
	var declared []protocol.SnapshotProcess
	for _, stage := range []string{"template_after_source_release", "idle", "touched", "executed"} {
		var boundary protocol.SnapshotDensityBoundary
		if err := decoder.Decode(&boundary); err != nil {
			t.Fatalf("decode %s: %v; stderr=%s", stage, err, stderr.String())
		}
		if err := protocol.ValidateSnapshotDensityBoundary(boundary); err != nil {
			t.Fatal(err)
		}
		if boundary.Stage != stage || boundary.Instances != count || boundary.Source.PID != uint32(cmd.Process.Pid) {
			t.Fatal("live boundary differs from owned source/request")
		}
		record := experiment.SnapshotDensityRecord{Boundary: boundary}
		refs := append([]protocol.SnapshotProcess{boundary.Source, boundary.Template}, boundary.Restored...)
		for i, ref := range refs {
			parent := boundary.Template.PID
			if i == 0 {
				parent = uint32(os.Getpid())
			} else if i == 1 {
				parent = boundary.Source.PID
			}
			r, err := collectors.CollectSnapshotProcess(ref, parent, origin)
			if err != nil {
				t.Fatal(err)
			}
			f, err := collectors.ValidateSnapshotProcessReading(r, ref, parent)
			if err != nil || f.Threads != 1 {
				t.Fatal("density child not held single-threaded", err)
			}
			record.Readings = append(record.Readings, r)
		}
		e.Records = append(e.Records, record)
		declared = refs
		if stage == "idle" && failAt == "source_killed_at_idle" {
			// Signal only this subprocess owned by exec.Cmd, never reported PIDs.
			if err := cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			input.Close()
			var extra json.RawMessage
			if err := decoder.Decode(&extra); err != io.EOF {
				t.Fatal("killed source emitted completion", err)
			}
			if err := cmd.Wait(); err == nil {
				t.Fatal("killed source succeeded")
			}
			waited = true
			if ctx.Err() != nil || cmd.ProcessState.Exited() {
				t.Fatal("source-death probe did not observe owned source signal")
			}
			return e, declared
		}
		if stage == failAt {
			if _, err := fmt.Fprintln(input, "invalid-continuation"); err != nil {
				t.Fatal(err)
			}
			input.Close()
			var extra json.RawMessage
			if err := decoder.Decode(&extra); err != io.EOF {
				t.Fatalf("failure must not produce success proof: %s, %v", extra, err)
			}
			if err := cmd.Wait(); err == nil {
				t.Fatal("invalid continuation accepted")
			}
			waited = true
			if ctx.Err() != nil {
				t.Fatal("cleanup required outer timeout", ctx.Err())
			}
			if !cmd.ProcessState.Exited() {
				t.Fatal("source did not exit normally through cleanup")
			}
			return e, declared
		}
		if _, err := fmt.Fprintln(input, "continue"); err != nil {
			t.Fatal(err)
		}
	}
	if err := decoder.Decode(&e.Proof); err != nil {
		t.Fatalf("final proof: %v; %s", err, stderr.String())
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatal("extra density output", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("native density failed: %v; %s", err, stderr.String())
	}
	waited = true
	if err := experiment.ValidateSnapshotDensityEvidence(e); err != nil {
		t.Fatal(err)
	}
	wantArch := map[string]string{"arm64": "aarch64", "amd64": "x86_64"}[runtime.GOARCH]
	if e.Proof.Architecture != wantArch {
		t.Fatal("density proof is not native")
	}
	return e, declared
}

func densityProcGoneError(err error) bool { return os.IsNotExist(err) || errors.Is(err, syscall.ESRCH) }

func TestDensityProcGoneError(t *testing.T) {
	for _, err := range []error{&os.PathError{Op: "read", Path: "/proc/1/stat", Err: syscall.ENOENT}, &os.PathError{Op: "read", Path: "/proc/1/stat", Err: syscall.ESRCH}} {
		if !densityProcGoneError(err) {
			t.Fatalf("disappeared process rejected: %v", err)
		}
	}
	for _, err := range []error{nil, syscall.EACCES, syscall.EIO} {
		if densityProcGoneError(err) {
			t.Fatalf("unreadable process incorrectly treated as gone: %v", err)
		}
	}
}

func densityProcessesGone(t *testing.T, refs []protocol.SnapshotProcess) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var remaining []protocol.SnapshotProcess
		for _, ref := range refs {
			data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", ref.PID))
			if densityProcGoneError(err) {
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			end := strings.LastIndex(string(data), ") ")
			if end < 0 {
				t.Fatal("invalid retained process stat")
			}
			fields := string(data)[end+2:]
			// Exact birth distinguishes PID reuse; never signal any of these refs.
			if f := strings.Fields(fields); len(f) >= 20 && f[19] == ref.StartTimeTicks {
				remaining = append(remaining, ref)
			}
		}
		if len(remaining) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("declared density processes still present, including zombies: %+v", remaining)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func saveDensityEvidence(t *testing.T, name string, value any) {
	t.Helper()
	root := os.Getenv("WASMBENCH_SNAPSHOT_DENSITY_EVIDENCE_DIR")
	if root == "" {
		return
	}
	f, err := os.OpenFile(filepath.Join(root, name+".json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = json.NewEncoder(f).Encode(value)
	closeErr := f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
}

func TestNativeSnapshotDensity(t *testing.T) {
	if runtime.GOOS != "linux" || os.Getenv("WASMBENCH_SNAPSHOT_DENSITY_ADAPTER") == "" {
		t.Skip("requires built native Linux snapshot worker")
	}
	for _, backend := range []string{"cranelift", "winch"} {
		for _, count := range []int{1, 2, 4, 8, 32} {
			t.Run(fmt.Sprintf("%s/%d", backend, count), func(t *testing.T) {
				e, refs := densityNative(t, backend, count, "")
				densityProcessesGone(t, refs)
				saveDensityEvidence(t, fmt.Sprintf("%s-%d", backend, count), e)
			})
		}
	}
}

func TestNativeSnapshotDensityCleanup(t *testing.T) {
	if runtime.GOOS != "linux" || os.Getenv("WASMBENCH_SNAPSHOT_DENSITY_ADAPTER") == "" {
		t.Skip("requires built native Linux snapshot worker")
	}
	for _, backend := range []string{"cranelift", "winch"} {
		for _, stage := range []string{"template_after_source_release", "idle", "touched", "executed", "source_killed_at_idle"} {
			t.Run(backend+"/"+stage, func(t *testing.T) {
				e, refs := densityNative(t, backend, 8, stage)
				densityProcessesGone(t, refs)
				if err := experiment.ValidateSnapshotDensityPrefix(e); err != nil {
					t.Fatal(err)
				}
				saveDensityEvidence(t, "cleanup-"+backend+"-"+stage, densityCleanupEvidence{"native-snapshot-density-cleanup-v1", stage, true, e})
			})
		}
	}
}

type densityCleanupEvidence struct {
	Version                  string                             `json:"version"`
	ExpectedFailure          string                             `json:"expected_failure"`
	AllDeclaredProcessesGone bool                               `json:"all_declared_processes_gone"`
	Prefix                   experiment.SnapshotDensityEvidence `json:"prefix"`
}

func TestRetainedSnapshotDensity(t *testing.T) {
	root := os.Getenv("WASMBENCH_SNAPSHOT_RETAINED_DENSITY_EVIDENCE_DIR")
	if root == "" {
		t.Skip("requires completed native density evidence")
	}
	for _, backend := range []string{"cranelift", "winch"} {
		for _, count := range []int{1, 2, 4, 8, 32} {
			data, err := os.ReadFile(filepath.Join(root, fmt.Sprintf("%s-%d.json", backend, count)))
			if err != nil {
				t.Fatal(err)
			}
			var e experiment.SnapshotDensityEvidence
			if err := json.Unmarshal(data, &e); err != nil {
				t.Fatal(err)
			}
			if e.Backend != backend || e.Instances != count {
				t.Fatal("retained density identity differs")
			}
			if err := experiment.ValidateSnapshotDensityEvidence(e); err != nil {
				t.Fatal(err)
			}
		}
		for _, stage := range []string{"template_after_source_release", "idle", "touched", "executed", "source_killed_at_idle"} {
			data, err := os.ReadFile(filepath.Join(root, "cleanup-"+backend+"-"+stage+".json"))
			if err != nil {
				t.Fatal(err)
			}
			var e densityCleanupEvidence
			if err := json.Unmarshal(data, &e); err != nil {
				t.Fatal(err)
			}
			wantRecords := map[string]int{"template_after_source_release": 1, "idle": 2, "touched": 3, "executed": 4, "source_killed_at_idle": 2}[stage]
			if e.Version != "native-snapshot-density-cleanup-v1" || e.ExpectedFailure != stage || !e.AllDeclaredProcessesGone || e.Prefix.Backend != backend || e.Prefix.Instances != 8 || len(e.Prefix.Records) != wantRecords {
				t.Fatal("invalid cleanup evidence identity/prefix")
			}
			if err := experiment.ValidateSnapshotDensityPrefix(e.Prefix); err != nil {
				t.Fatal(err)
			}
			if experiment.ValidateSnapshotDensityEvidence(e.Prefix) == nil {
				t.Fatal("failed prefix became completed density proof")
			}
		}
	}
}
