//go:build darwin || linux

package agent

import (
	"os/exec"
	"testing"
)

func TestProcessPeakRSSFromReapedChild(t *testing.T) {
	cmd := exec.Command("sh", "-c", "exit 0")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	bytes, ok := processPeakRSS(cmd.ProcessState)
	if !ok || bytes <= 0 {
		t.Fatalf("missing child peak RSS: %v, %v", bytes, ok)
	}
}
