package collectors

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestProfileBufferBound(t *testing.T) {
	b := profileBuffer{limit: 4}
	b.Write([]byte("1234"))
	b.Write([]byte("5"))
	b.Write([]byte("ignored"))
	if !b.overflow || string(b.data) != "1234" {
		t.Fatal(b)
	}
}

func TestGoCPUProfileOwnershipAndFormat(t *testing.T) {
	sum := sha256.Sum256([]byte("module"))
	module := hex.EncodeToString(sum[:])
	p := StartGoCPUProfile(module)
	defer p.Stop()
	nested := StartGoCPUProfile(module)
	n := nested.Stop()
	if n.Status != "unavailable" || n.Reason == "" || n.Validate(module) != nil {
		t.Fatal(n)
	}
	// Exercise the writer while the original profiler is still active. No
	// positive sampling count is assumed from a short scheduler-dependent test.
	until := time.Now().Add(40 * time.Millisecond)
	for time.Now().Before(until) {
		sum = sha256.Sum256(sum[:])
	}
	a := p.Stop()
	if a.Status != "collected" || a.Validate(module) != nil || p.Stop().SHA256 != a.SHA256 {
		t.Fatal(a.Status, a.Reason)
	}
	path := filepath.Join(t.TempDir(), "cpu.pprof")
	if err := os.WriteFile(path, a.Data, 0600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("go", "tool", "pprof", "-raw", path).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, output)
	}
	// Stop released the process-global profiler for the next independent pass.
	next := StartGoCPUProfile(module)
	if a := next.Stop(); a.Status != "collected" {
		t.Fatal(a)
	}
}
