package protocol

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestCPUProfileTransport(t *testing.T) {
	var b bytes.Buffer
	z := gzip.NewWriter(&b)
	z.Write([]byte("opaque protobuf transport"))
	z.Close()
	sum := sha256.Sum256(b.Bytes())
	digest := hex.EncodeToString(sum[:])
	p := CPUProfile{Version: 1, ModuleSHA256: digest, Format: "pprof-gzip", Collector: "runtime/pprof", CollectorVersion: "test", Scope: "adapter_process_go_cpu", Window: "run_request_including_setup_warmup_verification", Quality: "sampled", Status: "collected", SHA256: digest, Data: b.Bytes()}
	if err := p.Validate(digest); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*CPUProfile){func(p *CPUProfile) { p.Scope = "guest_only" }, func(p *CPUProfile) { p.Data = []byte("invalid") }, func(p *CPUProfile) { p.ModuleSHA256 = "bad" }, func(p *CPUProfile) { p.Status = "unavailable" }, func(p *CPUProfile) { p.Data = nil }} {
		bad := p
		mutate(&bad)
		if bad.Validate(digest) == nil {
			t.Fatal("invalid evidence accepted")
		}
	}
	p.Status, p.Reason, p.SHA256, p.Data = "unavailable", "busy", "", nil
	if err := p.Validate(digest); err != nil {
		t.Fatal(err)
	}
}

func TestProfilingRequestContract(t *testing.T) {
	p := &Preparation{Profile: "profiling", Workload: Workload{ABI: "core", Reset: "stateless", Export: "run", Oracle: Oracle{Kind: "exact_u64"}}}
	r := &RunRequest{Scenario: "steady", Samples: 2, Operations: 3, Warmup: 1}
	if err := ValidateProfilingRun(p, r); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*RunRequest){func(r *RunRequest) { r.PhaseBarriers = true }, func(r *RunRequest) { r.Scenario = "compile" }, func(r *RunRequest) { r.Warmup = -1 }, func(r *RunRequest) { r.Operations = 0 }, func(r *RunRequest) { r.Samples = 100001 }} {
		bad := *r
		mutate(&bad)
		if ValidateProfilingRun(p, &bad) == nil {
			t.Fatal(bad)
		}
	}
	p.Workload.Reset = "fresh_instance_per_sample"
	if ValidateProfilingRun(p, r) == nil {
		t.Fatal("reset contract ignored")
	}
}
