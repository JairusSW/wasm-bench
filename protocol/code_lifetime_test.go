package protocol

import (
	"encoding/json"
	"testing"
)

func lifetimeFixture() (CodeLifetime, CodeImage) {
	c := codeFixture()
	c.Version = 2
	c.Backend = "cranelift"
	c.FunctionAttribution = "engine_reported"
	c.Functions = []CodeFunction{{WasmIndex: 0, Length: 2, Tier: "cranelift"}, {WasmIndex: 1, Offset: 2, Length: 2, Tier: "cranelift"}}
	l := CodeLifetime{Version: 1, Collector: "Wasmtime/CustomCodeMemory", CollectorVersion: "46.0.1", Scope: "published_executable_text_capacity", Quality: "engine_callback", ModuleSHA256: c.ModuleSHA256, ImageSHA256: c.SHA256, Backend: c.Backend, Architecture: c.Architecture, PageSize: 4096, Publication: 1, ReleasePolicy: "drop_module_handles_then_store_then_engine", BeforeDrop: Values{7}, AfterDrop: Values{7}, Events: []CodeLifetimeEvent{{Sequence: 0, ElapsedNS: 1, Publication: 1, Kind: "published", Address: "9007199254740992", Capacity: 4096, ActiveCapacity: 4096, CumulativeCapacity: 4096}, {Sequence: 1, ElapsedNS: 5, Publication: 1, Kind: "unpublished", Address: "9007199254740992", Capacity: 4096, CumulativeCapacity: 4096}}}
	for i, s := range []string{"compiled", "instance_verified", "module_handles_dropped", "store_dropped", "engine_dropped"} {
		count := uint64(1)
		active := uint64(4096)
		if i >= 3 {
			count = 2
			active = 0
		}
		l.Checkpoints = append(l.Checkpoints, CodeLifetimeCheckpoint{Stage: s, ElapsedNS: int64(i + 2), EventCount: count, ActiveCapacity: active, CumulativeCapacity: 4096})
	}
	return l, c
}

func TestCodeLifetimeExactRoundTrip(t *testing.T) {
	l, c := lifetimeFixture()
	if err := l.Validate(c.ModuleSHA256, &c); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(l)
	if err != nil {
		t.Fatal(err)
	}
	var decoded CodeLifetime
	if err = json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Events[0].Address != "9007199254740992" || decoded.BeforeDrop[0] != 7 {
		t.Fatal("wire precision lost")
	}
	if err = decoded.Validate(c.ModuleSHA256, &c); err != nil {
		t.Fatal(err)
	}
}

func TestCodeLifetimeRejectsDetachedOrInventedEvidence(t *testing.T) {
	mutations := map[string]func(*CodeLifetime, *CodeImage){
		"version":        func(l *CodeLifetime, c *CodeImage) { l.Version = 2 },
		"collector":      func(l *CodeLifetime, c *CodeImage) { l.CollectorVersion = "latest" },
		"scope":          func(l *CodeLifetime, c *CodeImage) { l.Scope = "total_emitted_native_code" },
		"quality":        func(l *CodeLifetime, c *CodeImage) { l.Quality = "physical_reclamation" },
		"module":         func(l *CodeLifetime, c *CodeImage) { l.ModuleSHA256 = "other" },
		"image":          func(l *CodeLifetime, c *CodeImage) { l.ImageSHA256 = "other" },
		"backend":        func(l *CodeLifetime, c *CodeImage) { l.Backend = "winch" },
		"page":           func(l *CodeLifetime, c *CodeImage) { l.PageSize = 4097 },
		"publication":    func(l *CodeLifetime, c *CodeImage) { l.Events[1].Publication = 2 },
		"clock":          func(l *CodeLifetime, c *CodeImage) { l.Events[1].ElapsedNS = 0 },
		"negative":       func(l *CodeLifetime, c *CodeImage) { l.Events[0].ElapsedNS = -1 },
		"precision":      func(l *CodeLifetime, c *CodeImage) { l.Events[1].ElapsedNS = 1 << 53 },
		"address":        func(l *CodeLifetime, c *CodeImage) { l.Events[1].Address = "4096" },
		"canonical":      func(l *CodeLifetime, c *CodeImage) { l.Events[0].Address = "04096" },
		"overflow":       func(l *CodeLifetime, c *CodeImage) { l.Events[0].Address = "18446744073709547520" },
		"zero":           func(l *CodeLifetime, c *CodeImage) { l.Events[0].Capacity = 0 },
		"capacity":       func(l *CodeLifetime, c *CodeImage) { l.Events[1].Capacity = 8192 },
		"active":         func(l *CodeLifetime, c *CodeImage) { l.Events[1].ActiveCapacity = 4096 },
		"cumulative":     func(l *CodeLifetime, c *CodeImage) { l.Events[1].CumulativeCapacity = 0 },
		"emission":       func(l *CodeLifetime, c *CodeImage) { l.Events[0].Kind = "emitted" },
		"offset":         func(l *CodeLifetime, c *CodeImage) { l.ImageOffset = 4094 },
		"retirement":     func(l *CodeLifetime, c *CodeImage) { l.Events = l.Events[:1] },
		"extra":          func(l *CodeLifetime, c *CodeImage) { l.Events = append(l.Events, l.Events[1]) },
		"order":          func(l *CodeLifetime, c *CodeImage) { l.Checkpoints[2].Stage = "store_dropped" },
		"prefix":         func(l *CodeLifetime, c *CodeImage) { l.Checkpoints[2].EventCount = 2 },
		"checkpoint":     func(l *CodeLifetime, c *CodeImage) { l.Checkpoints[3].ElapsedNS = 4 },
		"future":         func(l *CodeLifetime, c *CodeImage) { l.Checkpoints[2].ElapsedNS = 6 },
		"retained":       func(l *CodeLifetime, c *CodeImage) { l.Checkpoints[2].ActiveCapacity = 0 },
		"release":        func(l *CodeLifetime, c *CodeImage) { l.ReleasePolicy = "drop_module" },
		"oracle":         func(l *CodeLifetime, c *CodeImage) { l.AfterDrop[0] = 8 },
		"missing_oracle": func(l *CodeLifetime, c *CodeImage) { l.BeforeDrop = nil; l.AfterDrop = nil },
		"legacy":         func(l *CodeLifetime, c *CodeImage) { c.Version = 1 },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			l, c := lifetimeFixture()
			mutate(&l, &c)
			if l.Validate(c.ModuleSHA256, &c) == nil {
				t.Fatal("invalid code lifetime evidence accepted")
			}
		})
	}
}

func TestCodeLifetimeRequestExcludesTimingAndCompoundContracts(t *testing.T) {
	p := Preparation{Profile: "code", Workload: Workload{ABI: "core", Reset: "stateless", Export: "run", Oracle: Oracle{Kind: "exact_u64", Expected: Values{7}}}}
	r := RunRequest{Scenario: "code-lifetime", Samples: 1, Operations: 1}
	if err := ValidateCodeLifetimeRun(&p, &r); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"timing", "samples", "operations", "warmup", "barriers", "sustained", "host", "pointer", "continuation", "oracle"} {
		t.Run(mode, func(t *testing.T) {
			p, r := p, r
			switch mode {
			case "timing":
				p.Profile = "timing"
			case "samples":
				r.Samples = 2
			case "operations":
				r.Operations = 2
			case "warmup":
				r.Warmup = 1
			case "barriers":
				r.PhaseBarriers = true
			case "sustained":
				r.SustainedDurationNS = 1
			case "host":
				p.Workload.HostProfile = "identity-v1"
			case "pointer":
				p.Workload.Oracle.OutputPointerExport = "ptr"
			case "continuation":
				p.Workload.Continuation = &ContinuationContract{}
			case "oracle":
				p.Workload.Oracle.Expected = nil
			}
			if ValidateCodeLifetimeRun(&p, &r) == nil {
				t.Fatal("invalid diagnostic request accepted")
			}
		})
	}
}
