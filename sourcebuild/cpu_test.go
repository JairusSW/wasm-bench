//go:build linux || darwin

package sourcebuild

import (
	"context"
	"math"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestCPUAccounting(t *testing.T) {
	missing := captureCPU(nil)
	if missing.validate() != nil || missing.Status != "unavailable" || missing.TotalNS != nil {
		t.Fatal(missing)
	}
	z := int64(0)
	zero := captureCPU(nil)
	zero.Status, zero.Reason = "available", ""
	zero.UserNS, zero.SystemNS, zero.TotalNS = &z, &z, &z
	if zero.validate() != nil || summedCPU([]StepResult{{CPU: zero}}) == nil || *summedCPU([]StepResult{{CPU: zero}}) != 0 {
		t.Fatal("lost real zero")
	}
	if summedCPU([]StepResult{{CPU: missing}}) != nil {
		t.Fatal("missing became zero")
	}
	max, one := int64(math.MaxInt64), int64(1)
	overflow := *zero
	overflow.UserNS, overflow.SystemNS, overflow.TotalNS = &max, &one, &max
	if overflow.validate() == nil {
		t.Fatal("CPU sum overflow accepted")
	}
	negative := int64(-1)
	bad := *zero
	bad.UserNS = &negative
	if bad.validate() == nil {
		t.Fatal("negative CPU accepted")
	}
	cmd := exec.Command("/bin/sh", "-c", "i=0; while [ \"$i\" -lt 100000 ]; do i=$((i+1)); done")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	got := captureCPU(cmd.ProcessState)
	if got.validate() != nil || got.Status != "available" || *got.TotalNS <= 0 {
		t.Fatal("missing live CPU usage", got)
	}
}

func TestCPUOnlyInDedicatedBuildProfile(t *testing.T) {
	r, dir, a := fixture(t)
	l, err := Pin(r, dir, a)
	if err != nil {
		t.Fatal(err)
	}
	for _, profile := range []string{"", "timing", "cpu"} {
		out := filepath.Join(dir, "build-"+profile)
		result, err := buildWithProfile(context.Background(), l, out, time.Second*10, "", "", profile)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Verify(out); err != nil {
			t.Fatal(err)
		}
		if profile == "cpu" {
			if result.Steps[0].CPU == nil || result.Steps[0].CPU.validate() != nil {
				t.Fatal("missing CPU observation")
			}
		} else if result.Steps[0].CPU != nil {
			t.Fatal("instrumented timing build")
		}
	}
}
