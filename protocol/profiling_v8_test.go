package protocol

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

func TestV8ProfileEnvelope(t *testing.T) {
	p := CPUProfile{Version: 1, ModuleSHA256: strings.Repeat("a", 64), Format: "v8-cpuprofile-json", Collector: "node:inspector/Profiler", CollectorVersion: "fixture", Scope: "adapter_v8_isolate_sampled_stacks", Window: "run_request_including_setup_warmup_verification", Quality: "sampled", Status: "collected"}
	for _, tt := range []struct {
		data  string
		valid bool
	}{
		{`{"nodes":[],"startTime":0,"endTime":1}`, true},
		{`{"nodes":null,"startTime":0,"endTime":1}`, false},
		{`{"nodes":{},"startTime":0,"endTime":1}`, false},
		{`{"nodes":[],"startTime":2,"endTime":1}`, false},
		{`{"nodes":[],"startTime":null,"endTime":1}`, false},
		{`null`, false}, {`not json`, false},
	} {
		p.Data = []byte(tt.data)
		p.SHA256 = fmt.Sprintf("%x", sha256.Sum256(p.Data))
		if err := p.Validate(p.ModuleSHA256); (err == nil) != tt.valid {
			t.Fatalf("%s: %v", tt.data, err)
		}
	}
	p.Data = []byte(`{"nodes":[],"startTime":0,"endTime":1}`)
	p.SHA256 = fmt.Sprintf("%x", sha256.Sum256(p.Data))
	if p.Extension() != ".cpuprofile" {
		t.Fatal(p.Extension())
	}
	p.Scope = "adapter_process_go_cpu"
	if p.Validate(p.ModuleSHA256) == nil {
		t.Fatal("cross-collector scope accepted")
	}
	p.Scope = "adapter_v8_isolate_sampled_stacks"
	p.SHA256 = strings.Repeat("b", 64)
	if p.Validate(p.ModuleSHA256) == nil {
		t.Fatal("corrupt digest accepted")
	}
	p.Status = "unavailable"
	p.Reason = "collector disabled"
	p.Data = nil
	p.SHA256 = ""
	if err := p.Validate(p.ModuleSHA256); err != nil {
		t.Fatal(err)
	}
}
