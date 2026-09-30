package experiment_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func TestV8SustainedProtocol(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js required for V8 integration")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := corpus.Generate(t.TempDir(), "sustained")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"timing", "memory", "no-hidden-precall", "middle-wrong", "unsupported-collection", "invalid-budget"} {
		t.Run(mode, func(t *testing.T) {
			c, err := agent.Start(context.Background(), []string{node, filepath.Join(root, "adapters/v8/adapter.mjs")}, filepath.Join(t.TempDir(), "log"), 15*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			d, err := c.Call(protocol.Request{Method: "describe"})
			if err != nil || !d.Description.Capabilities["can_sustained_execution"] || d.Description.Capabilities["can_sustained_post_collection"] || d.Description.Configuration["sustained_policy"] == "" {
				t.Fatal(d, err)
			}
			w := ws[0]
			r := protocol.RunRequest{Scenario: "sustained", Samples: 2, Operations: 100, Warmup: 1, SustainedDurationNS: 1000000}
			profile := "timing"
			bad := false
			switch mode {
			case "memory":
				profile = "memory"
			case "no-hidden-precall", "middle-wrong":
				fixture := "trajectory-limit.wasm"
				r.Operations = 1
				r.Warmup = 3
				if mode == "middle-wrong" {
					fixture = "counter-batch-wrong.wasm"
					r.Operations = 3
					r.Warmup = 0
					bad = true
				}
				w = protocol.Workload{ABI: "core", Reset: "stateless", Export: "run", Args: protocol.Values{}, Oracle: protocol.Oracle{Kind: "exact_u64", Expected: protocol.Values{7}}, Artifact: filepath.Join(root, "corpus/testdata", fixture)}
				w.SHA256, err = experiment.DigestFile(w.Artifact)
				if err != nil {
					t.Fatal(err)
				}
			case "unsupported-collection":
				profile = "memory"
				r.SustainedPostCollection = true
				bad = true
			case "invalid-budget":
				r.SustainedDurationNS = 0
				bad = true
			}
			_, err = c.Call(protocol.Request{Method: "prepare", Prepare: &protocol.Preparation{Workload: w, Artifact: w.Artifact, ArtifactSHA256: w.SHA256, Profile: profile}})
			if err != nil {
				t.Fatal(err)
			}
			resp, err := c.Call(protocol.Request{Method: "run", Run: &r})
			if bad {
				if err == nil {
					t.Fatal("invalid contract/result accepted", resp)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := protocol.VerifySustainedSequence(w, r, resp.Samples); err != nil {
				t.Fatal(err)
			}
			for i, s := range resp.Samples {
				if profile == "timing" && len(s.Observations) > 0 {
					t.Fatal("instrumented timing")
				}
				if profile == "memory" && len(s.Observations) < 2 {
					t.Fatal("missing V8 heap")
				}
				if s.Index != i || s.Warmup != (i < r.Warmup) {
					t.Fatal("sequence mismatch")
				}
			}
			release := resp.Samples[len(resp.Samples)-1].SustainedRelease
			if release.Policy != "js_references_dropped" || release.Closed || release.PostCollection != nil {
				t.Fatal("invented engine close or reclamation", release)
			}
		})
	}
}
