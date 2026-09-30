package protocol

import "fmt"

const GuestDensityPhase = "guest-density/provision_window"
const GuestDensityDenominator = "instance_group_instantiate_initialize_or_eager_restore_optional_first_write_excluding_setup_verification_release"

func GuestDensityGenerator(d GuestDensityContract) string {
	state := "unchanged"
	if d.FirstWrite {
		state = "first_write"
	}
	return fmt.Sprintf("wasmbench-guest-density-v1/%s/%s/%d-pages", d.Provisioning, state, d.Pages)
}

type GuestDensityContract struct {
	Instances    int    `json:"instances"`
	Pages        uint32 `json:"pages"`
	Provisioning string `json:"provisioning"`
	FirstWrite   bool   `json:"first_write"`
}

type GuestDensityInstance struct {
	MemorySHA256 string `json:"memory_sha256"`
	Global       uint32 `json:"global"`
	Checksum     uint64 `json:"checksum"`
}

type GuestDensityResult struct {
	Provisioning      string                 `json:"provisioning"`
	PayloadBytes      uint64                 `json:"payload_bytes"`
	SourceIndependent bool                   `json:"source_independent"`
	PayloadUnchanged  bool                   `json:"payload_unchanged"`
	Instances         []GuestDensityInstance `json:"instances"`
}

func ValidateGuestDensityWorkload(w Workload) error {
	d := w.GuestDensity
	if d == nil || d.Instances < 1 || d.Instances > 128 || d.Pages < 1 || d.Pages > 4 || (d.Provisioning != "fresh_initialize" && d.Provisioning != "eager_guest_restore") {
		return fmt.Errorf("guest density requires 1 to 128 instances, 1 to 4 pages and an explicit provisioning policy")
	}
	want := uint64(d.Pages)*65536*7 + 42
	if d.FirstWrite {
		want += 5
	}
	if w.ABI != "core" || w.HostProfile != "" || w.Input != nil || w.Command != nil || w.Vectors != nil || w.Density != nil || w.Checkpoint != nil || len(w.Args) != 0 || w.Export != "benchmark" || w.Initialize != "" || w.Reset != "fresh_instance_per_sample" || w.WorkUnit != "instance_group" || w.Units != 1 || w.Generator != GuestDensityGenerator(*d) || w.Dimension != "instances" || w.Size != d.Instances || w.Oracle.Kind != "guest_density_v1" || w.Oracle.Float != nil || len(w.Oracle.Memory) != 0 || w.Oracle.OutputPointerExport != "" || w.Oracle.ExpectedTrap != "" || len(w.Oracle.Expected) != 1 || w.Oracle.Expected[0] != want {
		return fmt.Errorf("invalid guest density execution contract")
	}
	return nil
}

func ValidateGuestDensity(p *Preparation, r *RunRequest) error {
	if p == nil || r == nil {
		return fmt.Errorf("missing guest density preparation/request")
	}
	if err := ValidateGuestDensityWorkload(p.Workload); err != nil {
		return err
	}
	if r.Scenario != "guest-density" || (p.Profile != "timing" && p.Profile != "memory") || (r.PhaseBarriers && p.Profile != "memory") || r.Samples < 1 || r.Samples > 10000 || r.Operations != 1 || r.Warmup != 0 {
		return fmt.Errorf("guest density requires single-operation timing/memory samples without warmup; barriers require memory")
	}
	return nil
}

func VerifyGuestDensitySample(w Workload, s Sample) error {
	if err := ValidateGuestDensityWorkload(w); err != nil {
		return err
	}
	d := w.GuestDensity
	e := s.GuestDensityResult
	if e == nil || !s.Verified || s.Warmup || s.ElapsedNS < 0 || s.Operations != 1 || s.SampleType != "individual_operation" || len(s.Result) != 1 || s.Result[0] != w.Oracle.Expected[0] || e.Provisioning != d.Provisioning || len(e.Instances) != d.Instances || s.CheckpointResult != nil || s.TrapResult != nil || s.CommandResult != nil || s.SustainedWindow != nil || s.SustainedRelease != nil {
		return fmt.Errorf("incorrect guest density sample identity/budget")
	}
	if d.Provisioning == "eager_guest_restore" {
		if e.PayloadBytes != uint64(d.Pages)*65536+4 || !e.SourceIndependent || !e.PayloadUnchanged {
			return fmt.Errorf("incorrect guest density copy isolation/payload")
		}
	} else if e.PayloadBytes != 0 || e.SourceIndependent || e.PayloadUnchanged {
		return fmt.Errorf("fresh density invented checkpoint evidence")
	}
	global := uint32(42)
	if d.FirstWrite {
		global = 43
	}
	digest := CheckpointMemorySHA256(d.Pages, d.FirstWrite)
	for _, v := range e.Instances {
		if v.Global != global || v.MemorySHA256 != digest || v.Checksum != w.Oracle.Expected[0] {
			return fmt.Errorf("incorrect guest density instance state")
		}
	}
	return nil
}
