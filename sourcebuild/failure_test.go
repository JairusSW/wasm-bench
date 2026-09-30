package sourcebuild

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wasmbench/wasmbench/agent"
)

func TestFailedBuildEvidence(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "step-000.log"), []byte("killed"), 0600); err != nil {
		t.Fatal(err)
	}
	l := Lock{Recipe: Recipe{Steps: []Step{{Tool: "compiler"}}}}
	c := BenchmarkConfig{Profile: "memory", Resources: &agent.ResourcePolicy{CgroupParent: "/delegated"}}
	s := memoryStep(1024)
	s.Tool, s.Log = "compiler", "step-000.log"
	s.Resources.OOM = true
	s.ContextError = "deadline_exceeded"
	r := Result{Schema: 1, Kind: "source_build", Lock: l, CollectionProfile: c.Profile, ResourcePolicy: c.Resources, Steps: []StepResult{s}}
	if err := verifyFailedBuild(root, ".", r, l, c, "oom"); err != nil {
		t.Fatal(err)
	}
	if !s.Resources.OOM {
		t.Fatal("verification mutated receipt")
	}
	if verifyFailedBuild(root, ".", r, l, c, "build_failed") == nil {
		t.Fatal("lost OOM accepted")
	}
	s.Resources.Observations[0].Unit = "ns"
	if verifyFailedBuild(root, ".", r, l, c, "oom") == nil {
		t.Fatal("invalid metric accepted")
	}
	r.Steps[0].Resources = &agent.ToolExecution{}
	r.Steps[0].ContextError = ""
	r.Steps[0].ElapsedNS = 0
	if err := verifyFailedBuild(root, ".", r, l, c, "build_failed"); err != nil {
		t.Fatal(err)
	}
	if verifyFailedBuild(root, ".", r, l, c, "oom") == nil {
		t.Fatal("invented OOM accepted")
	}
	r.Steps[0].Log = "missing.log"
	if verifyFailedBuild(root, ".", r, l, c, "build_failed") == nil {
		t.Fatal("missing log accepted")
	}
}
