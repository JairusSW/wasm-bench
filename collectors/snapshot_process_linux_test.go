package collectors

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/protocol"
)

type nativeProcTree struct{ Template, Restored protocol.SnapshotProcess }

func selfProcRef(t *testing.T) protocol.SnapshotProcess {
	t.Helper()
	raw, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		t.Fatal(err)
	}
	s, err := parseSnapshotStat(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	return protocol.SnapshotProcess{PID: s.pid, StartTimeTicks: s.birth}
}
func procHelper(t *testing.T, ctx context.Context, role string) (*exec.Cmd, io.WriteCloser, *bufio.Reader) {
	t.Helper()
	c := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSnapshotProcessHelper$")
	c.Env = append(os.Environ(), "WASMBENCH_PROC_HELPER="+role)
	c.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	in, err := c.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	c.Stderr = os.Stderr
	// Linux's parent-death signal is tied to the spawning thread, not just its
	// process. Keep that Go thread alive until its owned child has been reaped.
	runtime.LockOSThread()
	if err := c.Start(); err != nil {
		runtime.UnlockOSThread()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		in.Close()
		if c.ProcessState == nil {
			c.Process.Kill()
			c.Wait()
		}
		runtime.UnlockOSThread()
	})
	return c, in, bufio.NewReader(out)
}
func TestSnapshotProcessHelper(t *testing.T) {
	role := os.Getenv("WASMBENCH_PROC_HELPER")
	if role == "" {
		t.Skip("owned subprocess helper")
	}
	self := selfProcRef(t)
	if role == "restored" {
		if err := json.NewEncoder(os.Stdout).Encode(self); err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, os.Stdin)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, in, out := procHelper(t, ctx, "restored")
	line, err := out.ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var child protocol.SnapshotProcess
	if err := json.Unmarshal(line, &child); err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(nativeProcTree{self, child}); err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, os.Stdin)
	in.Close()
	if err := c.Wait(); err != nil {
		t.Fatal(err)
	}
}
func TestNativeSnapshotProcessLineage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, in, out := procHelper(t, ctx, "template")
	line, err := out.ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var tree nativeProcTree
	if err := json.Unmarshal(line, &tree); err != nil {
		t.Fatal(err)
	}
	self := selfProcRef(t)
	if tree.Template.PID != uint32(c.Process.Pid) {
		t.Fatal("helper identity differs from launched process")
	}
	origin := time.Now()
	for index, item := range []struct {
		ref    protocol.SnapshotProcess
		parent uint32
	}{{self, uint32(os.Getppid())}, {tree.Template, self.PID}, {tree.Restored, tree.Template.PID}} {
		r, err := CollectSnapshotProcess(item.ref, item.parent, origin)
		if err != nil {
			t.Fatal(err)
		}
		v, err := ValidateSnapshotProcessReading(r, item.ref, item.parent)
		if err != nil {
			t.Fatal(err)
		}
		if v.RSSBytes == 0 || v.VirtualBytes == 0 {
			t.Fatal("missing live footprint")
		}
		t.Logf("verified PID=%d parent=%d birth=%s RSS=%d smaps=%s", item.ref.PID, item.parent, item.ref.StartTimeTicks, v.RSSBytes, r.SmapsStatus)
		if root := os.Getenv("WASMBENCH_SNAPSHOT_PROC_EVIDENCE_DIR"); root != "" {
			role := []string{"source", "template", "restored"}[index]
			f, err := os.OpenFile(filepath.Join(root, role+".json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
			if err != nil {
				t.Fatal(err)
			}
			err = json.NewEncoder(f).Encode(r)
			closeErr := f.Close()
			if err != nil {
				t.Fatal(err)
			}
			if closeErr != nil {
				t.Fatal(closeErr)
			}
		}
		wrong := item.ref
		wrong.StartTimeTicks = "1"
		if _, err := CollectSnapshotProcess(wrong, item.parent, origin); err == nil {
			t.Fatal("accepted wrong live birth")
		}
		if _, err := CollectSnapshotProcess(item.ref, 0, origin); err == nil {
			t.Fatal("accepted wrong live parent")
		}
	}
	in.Close()
	if err := c.Wait(); err != nil {
		t.Fatal(err)
	}
	if _, err := CollectSnapshotProcess(tree.Template, self.PID, origin); err == nil {
		t.Fatal("accepted reaped process")
	}
}
