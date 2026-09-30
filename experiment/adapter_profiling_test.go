package experiment_test

import (
	"context"
	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBuiltAdapterCPUProfiling(t *testing.T) {
	ids := []string{}
	if os.Getenv("WASMBENCH_GO_PROFILE_TEST") == "1" {
		ids = append(ids, "wazero-interpreter", "wazero")
	}
	if os.Getenv("WASMBENCH_V8_PROFILE_TEST") == "1" {
		ids = append(ids, "v8")
	}
	if len(ids) == 0 {
		t.Skip("build wazero adapter and set WASMBENCH_GO_PROFILE_TEST=1")
	}
	root, _ := filepath.Abs("..")
	rts, err := experiment.ResolveRuntimes(root, ids)
	if err != nil {
		t.Fatal(err)
	}
	ws, err := corpus.Generate(t.TempDir(), "core")
	if err != nil {
		t.Fatal(err)
	}
	for _, rt := range rts {
		for _, mode := range []string{"valid", "wrong-oracle", "barriers"} {
			t.Run(rt.ID+"/"+mode, func(t *testing.T) {
				w := ws[0]
				if mode == "wrong-oracle" {
					w.Oracle.Expected = protocol.Values{999}
				}
				c, err := agent.Start(context.Background(), rt.Command, filepath.Join(t.TempDir(), "log"), 15*time.Second)
				if err != nil {
					t.Fatal(err)
				}
				defer c.Close()
				_, err = c.Call(protocol.Request{Method: "prepare", Prepare: &protocol.Preparation{Artifact: w.Artifact, ArtifactSHA256: w.SHA256, Workload: w, Profile: "profiling"}})
				if err != nil {
					t.Fatal(err)
				}
				r := &protocol.RunRequest{Scenario: "steady", Samples: 2, Operations: 10, Warmup: 1, PhaseBarriers: mode == "barriers"}
				resp, err := c.Call(protocol.Request{Method: "run", Run: r})
				if rt.ID == "wazero" || mode == "barriers" {
					if err == nil || resp.CPUProfile != nil {
						t.Fatal("unsupported profiling accepted", resp, err)
					}
					return
				}
				if resp.CPUProfile == nil || resp.CPUProfile.Status != "collected" || resp.CPUProfile.Validate(w.SHA256) != nil {
					t.Fatal("missing profile evidence", err)
				}
				if mode == "wrong-oracle" {
					if err == nil {
						t.Fatal("incorrect result accepted")
					}
					return
				}
				if err != nil || len(resp.Samples) != 3 {
					t.Fatal(resp, err)
				}
				for _, s := range resp.Samples {
					if !s.Verified || len(s.Observations) != 0 {
						t.Fatal(s)
					}
				}
			})
		}
	}
}
