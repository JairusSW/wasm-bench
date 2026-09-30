package agent

import (
	"context"
	"github.com/wasmbench/wasmbench/protocol"
	"path/filepath"
	"testing"
	"time"
)

func TestDeadlineTerminatesSilentAdapter(t *testing.T) {
	start := time.Now()
	c, e := Start(context.Background(), []string{"sleep", "10"}, filepath.Join(t.TempDir(), "stderr"), 50*time.Millisecond)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	if _, e = c.Call(protocol.Request{Method: "describe"}); e == nil {
		t.Fatal("silent process supplied a response")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("deadline failed")
	}
}
