package experiment_test

import (
	"context"
	"encoding/json"
	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestV8TierDiagnosticNoHiddenCalls(t *testing.T) {
	if os.Getenv("WASMBENCH_V8_TIER_TEST") != "1" {
		t.Skip("set WASMBENCH_V8_TIER_TEST=1 with supported Node/V8")
	}
	root, _ := filepath.Abs("..")
	rts, err := experiment.ResolveRuntimes(root, []string{"v8-tier-observed"})
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"trajectory", "admission", "wrong-oracle", "guest-trap", "timing-refusal"} {
		t.Run(mode, func(t *testing.T) {
			c, err := agent.Start(context.Background(), rts[0].Command, filepath.Join(t.TempDir(), "log"), 15*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			d, err := c.Call(protocol.Request{Method: "describe"})
			if err != nil || !d.Description.Capabilities["can_observe_tiers"] {
				t.Fatal(d, err)
			}
			artifact := filepath.Join(root, "corpus/testdata/trajectory-limit.wasm")
			digest, err := experiment.DigestFile(artifact)
			if err != nil {
				t.Fatal(err)
			}
			w := protocol.Workload{ABI: "core", Reset: "stateless", Export: "run", Args: protocol.Values{}, SHA256: digest, Oracle: protocol.Oracle{Kind: "exact_u64", Expected: protocol.Values{7}}}
			if mode == "wrong-oracle" {
				w.Oracle.Expected = protocol.Values{8}
			}
			profile := "profiling"
			r := protocol.RunRequest{Scenario: "trajectory", Samples: 3, Warmup: 2, Operations: 1}
			if mode == "admission" {
				profile = "timing"
				r = protocol.RunRequest{Scenario: "first-call", Samples: 1, Operations: 1}
			}
			if mode == "timing-refusal" {
				profile = "timing"
			}
			if mode == "guest-trap" {
				r.Samples = 6
				r.Warmup = 0
			}
			_, err = c.Call(protocol.Request{Method: "prepare", Prepare: &protocol.Preparation{Workload: w, Artifact: artifact, ArtifactSHA256: digest, Profile: profile}})
			if err != nil {
				t.Fatal(err)
			}
			resp, err := c.Call(protocol.Request{Method: "run", Run: &r})
			if mode == "wrong-oracle" || mode == "timing-refusal" || mode == "guest-trap" {
				if err == nil {
					t.Fatal("accepted invalid diagnostic", resp)
				}
				if mode == "wrong-oracle" || mode == "guest-trap" {
					count, outcome := 1, "oracle_mismatch"
					if mode == "guest-trap" {
						count, outcome = 6, "guest_trap"
					}
					if len(resp.Samples) != count {
						t.Fatal("lost failure prefix", resp)
					}
					for i, s := range resp.Samples {
						if s.TierWindow == nil || s.TierWindow.Validate(w, s) != nil || s.Verified != (i < count-1) {
							t.Fatal("invalid failure evidence", s)
						}
					}
					last := resp.Samples[count-1]
					if last.TierWindow.InvocationOutcome != outcome || last.TierWindow.FailureReason == "" {
						t.Fatal("lost terminal outcome", last)
					}
				}
				return
			}
			if err != nil || len(resp.Samples) != r.Samples+r.Warmup {
				t.Fatal(resp, err)
			}
			for _, s := range resp.Samples {
				if mode == "admission" {
					if s.TierWindow != nil {
						t.Fatal("instrumented sacrificial admission")
					}
					continue
				}
				if s.TierWindow == nil || s.TierWindow.Validate(w, s) != nil {
					t.Fatal("missing tier identity or bracket", s)
				}
				if s.Warmup != (s.Index < r.Warmup) {
					t.Fatal("lost explicit warmup")
				}
			}
		})
	}
}

func TestV8TierControllerRetainsGuestFailure(t *testing.T) {
	if os.Getenv("WASMBENCH_V8_TIER_TEST") != "1" {
		t.Skip("set WASMBENCH_V8_TIER_TEST=1")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	runtimes, err := experiment.ResolveRuntimes(root, []string{"v8-tier-observed"})
	if err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(root, "corpus/testdata/trajectory-limit.wasm")
	digest, err := experiment.DigestFile(artifact)
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately false stateless declaration: admission succeeds once but a
	// retained trajectory must catch the sixth-call trap. Not performance data.
	w := protocol.Workload{Schema: 1, ID: "adversarial/trajectory-limit", Artifact: artifact, SHA256: digest, ABI: "core", Features: []string{"mvp"}, Reset: "stateless", Export: "run", Args: protocol.Values{}, Oracle: protocol.Oracle{Kind: "exact_u64", Expected: protocol.Values{7}}, License: "MIT", WorkUnit: "invocation", Units: 1}
	lock, err := experiment.NewLock(experiment.Options{Suite: "adversarial-fixture", Profile: "profiling", Scenarios: []string{"trajectory"}, Launches: 1, Samples: 10, Warmup: 2, Operations: 1, Timeout: 15 * time.Second}, runtimes, []protocol.Workload{w})
	if err != nil {
		t.Fatal(err)
	}
	out, err := experiment.Run(context.Background(), lock, root, filepath.Join(t.TempDir(), "run"), func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	b, err := experiment.Load(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, trial := range b.Trials {
		if trial.Block < 0 {
			if trial.Status != "ok" || len(trial.Samples) != 1 {
				t.Fatal("admission warmed or rejected fixture", trial)
			}
			continue
		}
		if trial.Status != "error" || len(trial.Samples) != 6 || len(trial.AdapterSamples) != 0 {
			t.Fatal("failed trace lost or treated as unvalidated", trial)
		}
		for i, s := range trial.Samples {
			if s.Verified != (i < 5) || s.Warmup != (i < 2) {
				t.Fatal("failed call verified or warmup lost", s)
			}
		}
		last := trial.Samples[5]
		if last.TierWindow.InvocationOutcome != "guest_trap" || len(last.Result) != 0 {
			t.Fatal("trap masquerades as result", last)
		}
	}
}

func TestV8TierControllerRoundTrip(t *testing.T) {
	if os.Getenv("WASMBENCH_V8_TIER_TEST") != "1" {
		t.Skip("set WASMBENCH_V8_TIER_TEST=1")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	runtimes, err := experiment.ResolveRuntimes(root, []string{"v8-tier-observed"})
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	workloads, err := corpus.Generate(filepath.Join(tmp, "corpus"), "core")
	if err != nil {
		t.Fatal(err)
	}
	lock, err := experiment.NewLock(experiment.Options{Suite: "core", Profile: "profiling", Scenarios: []string{"trajectory"}, Launches: 1, Samples: 3, Warmup: 2, Operations: 1, Timeout: 15 * time.Second}, runtimes, workloads)
	if err != nil {
		t.Fatal(err)
	}
	out, err := experiment.Run(context.Background(), lock, tmp, filepath.Join(tmp, "run"), func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	b, err := experiment.Load(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Trials) != 2*len(workloads) {
		t.Fatal("lost admission or measured trials")
	}
	for _, trial := range b.Trials {
		if trial.Status != "ok" {
			t.Fatal(trial.ID, trial.Status, trial.Reason)
		}
		if trial.Block >= 0 {
			if len(trial.Samples) != 5 || trial.Samples[0].TierWindow == nil || trial.CPUProfile != nil {
				t.Fatal("lost dedicated tier diagnostics", trial)
			}
		} else if len(trial.Samples) != 1 || trial.Samples[0].TierWindow != nil {
			t.Fatal("admission was instrumented", trial)
		}
	}
	for _, trial := range b.Trials {
		if trial.Block < 0 {
			continue
		}
		path := filepath.Join(out, "trials", trial.ID+".json")
		original, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for name, change := range map[string]func(*experiment.Trial){
			"collector-version": func(x *experiment.Trial) { x.Samples[0].TierWindow.CollectorVersion = "forged" },
			"missing-window":    func(x *experiment.Trial) { x.Samples[0].TierWindow = nil },
			"changed-warmup":    func(x *experiment.Trial) { x.Samples[0].Warmup = false },
		} {
			t.Run(name, func(t *testing.T) {
				var changed experiment.Trial
				if err := json.Unmarshal(original, &changed); err != nil {
					t.Fatal(err)
				}
				change(&changed)
				encoded, err := json.Marshal(changed)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, encoded, 0644); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(filepath.Join(out, "checksums.json")); err != nil {
					t.Fatal(err)
				}
				if err := experiment.Seal(out); err != nil {
					t.Fatal(err)
				}
				if _, err := experiment.Load(out); err == nil {
					t.Fatal("resealed forged tier evidence accepted")
				}
				if err := os.WriteFile(path, original, 0644); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(filepath.Join(out, "checksums.json")); err != nil {
					t.Fatal(err)
				}
				if err := experiment.Seal(out); err != nil {
					t.Fatal(err)
				}
			})
		}
		break
	}
}
