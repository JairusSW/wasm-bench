package main

import (
	"github.com/wasmbench/wasmbench/protocol"
	"strings"
	"testing"
)

func TestCommandFailureIncludesBoundedStderrWithoutPerformanceCredit(t *testing.T) {
	c := &protocol.CommandContract{ExitCode: 0, OutputLimit: 16 << 20, StdoutSHA256: protocol.CommandDigest(nil)}
	stderr := []byte(strings.Repeat("failure\n", 1000))
	result, err := commandDigests(c, 1, nil, stderr)
	if err == nil || !strings.Contains(err.Error(), "command exit=1 expected=0") || !strings.Contains(err.Error(), "stderr prefix=") {
		t.Fatalf("missing failure diagnostic: %v", err)
	}
	if len(err.Error()) > 2200 {
		t.Fatal("unbounded stderr in diagnostic")
	}
	if result.StdoutSHA256 != "" {
		t.Fatal("incorrect command returned usable evidence")
	}
	_, err = commandDigests(c, 1, nil, nil)
	if err == nil || strings.Contains(err.Error(), "stderr prefix=") {
		t.Fatalf("empty stderr changed diagnostic: %v", err)
	}
	result, err = commandDigests(c, 0, nil, []byte("allowed warning"))
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("successful command changed: %v", err)
	}
}
