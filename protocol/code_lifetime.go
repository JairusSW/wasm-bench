package protocol

import (
	"fmt"
	"strconv"
)

// CodeLifetime observes executable-text publication, not total compiler
// emission, physical reclamation, or guest-instruction-only byte counts.
// Addresses are canonical decimal strings to avoid JavaScript rounding.
type CodeLifetime struct {
	Version          int                      `json:"version"`
	Collector        string                   `json:"collector"`
	CollectorVersion string                   `json:"collector_version"`
	Scope            string                   `json:"scope"`
	Quality          string                   `json:"quality"`
	ModuleSHA256     string                   `json:"module_sha256"`
	ImageSHA256      string                   `json:"image_sha256"`
	Backend          string                   `json:"backend"`
	Architecture     string                   `json:"architecture"`
	PageSize         uint64                   `json:"page_size"`
	BTI              bool                     `json:"bti"`
	Publication      uint64                   `json:"publication"`
	ImageOffset      uint64                   `json:"image_offset"`
	ReleasePolicy    string                   `json:"release_policy"`
	BeforeDrop       Values                   `json:"result_before_drop"`
	AfterDrop        Values                   `json:"result_after_drop"`
	Events           []CodeLifetimeEvent      `json:"events"`
	Checkpoints      []CodeLifetimeCheckpoint `json:"checkpoints"`
}

type CodeLifetimeEvent struct {
	Sequence           uint64 `json:"sequence"`
	ElapsedNS          int64  `json:"elapsed_ns"`
	Publication        uint64 `json:"publication"`
	Kind               string `json:"kind"`
	Address            string `json:"address"`
	Capacity           uint64 `json:"capacity"`
	ActiveCapacity     uint64 `json:"active_capacity"`
	CumulativeCapacity uint64 `json:"cumulative_published_capacity"`
}

type CodeLifetimeCheckpoint struct {
	Stage              string `json:"stage"`
	ElapsedNS          int64  `json:"elapsed_ns"`
	EventCount         uint64 `json:"event_count"`
	ActiveCapacity     uint64 `json:"active_capacity"`
	CumulativeCapacity uint64 `json:"cumulative_published_capacity"`
}

func ValidateCodeLifetimeRun(p *Preparation, r *RunRequest) error {
	if p == nil || r == nil || r.Scenario != "code-lifetime" {
		return fmt.Errorf("code lifetime requires an explicit diagnostic scenario")
	}
	copy := *r
	copy.Scenario = "compile-materialized"
	if err := ValidateMaterializedRun(p, &copy); err != nil {
		return err
	}
	w := p.Workload
	if w.HostProfile != "" || w.Continuation != nil || w.Oracle.OutputPointerExport != "" || r.SustainedDurationNS != 0 || r.SustainedPostCollection {
		return fmt.Errorf("code lifetime requires a stateless core result without host imports or compound state")
	}
	if len(w.Oracle.Expected) == 0 {
		return fmt.Errorf("code lifetime requires a nonempty exact result oracle")
	}
	return nil
}

func (l CodeLifetime) Validate(module string, image *CodeImage) error {
	const safe = uint64(1<<53 - 1)
	if image == nil {
		return fmt.Errorf("code lifetime omitted attributed native image")
	}
	if err := image.Validate(module); err != nil {
		return err
	}
	if l.Version != 1 || l.Collector != "Wasmtime/CustomCodeMemory" || l.CollectorVersion != "46.0.1" || l.Scope != "published_executable_text_capacity" || l.Quality != "engine_callback" || l.ModuleSHA256 != module || l.ImageSHA256 != image.SHA256 || l.Backend != image.Backend || l.Architecture != image.Architecture || image.Version != 2 || image.FunctionAttribution != "engine_reported" || len(image.Functions) == 0 || len(image.Functions) > 10000 || l.ReleasePolicy != "drop_module_handles_then_store_then_engine" || l.Publication != 1 || len(l.Events) != 2 || len(l.Checkpoints) != 5 || len(l.BeforeDrop) == 0 || len(l.BeforeDrop) != len(l.AfterDrop) {
		return fmt.Errorf("invalid code lifetime identity or coverage")
	}
	if l.Backend != "cranelift" && l.Backend != "winch" || l.PageSize < 4096 || l.PageSize > 1<<20 || l.PageSize&(l.PageSize-1) != 0 || l.BTI && l.Architecture != "arm64" {
		return fmt.Errorf("invalid code lifetime platform policy")
	}
	for i, v := range l.BeforeDrop {
		if v != l.AfterDrop[i] {
			return fmt.Errorf("code lifetime post-drop result changed")
		}
	}
	for i, e := range l.Events {
		address, err := strconv.ParseUint(e.Address, 10, 64)
		if err != nil || address == 0 || strconv.FormatUint(address, 10) != e.Address || address%l.PageSize != 0 || e.Capacity == 0 || e.Capacity > safe || e.Capacity%l.PageSize != 0 || address > ^uint64(0)-e.Capacity || e.ElapsedNS < 0 || uint64(e.ElapsedNS) > safe || e.Sequence != uint64(i) || e.Publication != l.Publication {
			return fmt.Errorf("invalid code lifetime event range or clock")
		}
	}
	first, last := l.Events[0], l.Events[1]
	if first.Kind != "published" || last.Kind != "unpublished" || first.Address != last.Address || first.Capacity != last.Capacity || first.ElapsedNS > last.ElapsedNS || first.ActiveCapacity != first.Capacity || last.ActiveCapacity != 0 || first.CumulativeCapacity != first.Capacity || last.CumulativeCapacity != first.Capacity || l.ImageOffset > first.Capacity || uint64(len(image.Data)) > first.Capacity-l.ImageOffset {
		return fmt.Errorf("invalid code publication/retirement or image binding")
	}
	names := []string{"compiled", "instance_verified", "module_handles_dropped", "store_dropped", "engine_dropped"}
	for i, c := range l.Checkpoints {
		count := uint64(1)
		if i >= 3 {
			count = 2
		}
		active := first.Capacity
		if i >= 3 {
			active = 0
		}
		if c.Stage != names[i] || c.EventCount != count || c.ActiveCapacity != active || c.CumulativeCapacity != first.Capacity || c.ElapsedNS < l.Events[count-1].ElapsedNS || uint64(c.ElapsedNS) > safe || i > 0 && c.ElapsedNS < l.Checkpoints[i-1].ElapsedNS || count == 1 && c.ElapsedNS > last.ElapsedNS {
			return fmt.Errorf("invalid code lifetime ownership checkpoint")
		}
	}
	return nil
}
