package protocol

import (
	"encoding/hex"
	"fmt"
)

// TierWindow brackets non-atomic, engine-reported code classifications around
// one exported-function invocation. It is not an execution trace or proof of
// the tier executed throughout the call, internal callees or compiler activity.
type TierWindow struct {
	InvocationOutcome string      `json:"invocation_outcome,omitempty"`
	FailureReason     string      `json:"failure_reason,omitempty"`
	Version           int         `json:"version"`
	ModuleSHA256      string      `json:"module_sha256"`
	Export            string      `json:"export"`
	Collector         string      `json:"collector"`
	CollectorVersion  string      `json:"collector_version"`
	Scope             string      `json:"scope"`
	Quality           string      `json:"quality"`
	Invocation        int         `json:"invocation"`
	Before            TierReading `json:"before"`
	OperationStartNS  int64       `json:"operation_start_ns"`
	OperationEndNS    int64       `json:"operation_end_ns"`
	After             TierReading `json:"after"`
}

type TierReading struct {
	StartNS int64  `json:"start_ns"`
	EndNS   int64  `json:"end_ns"`
	State   string `json:"state"`
	Reason  string `json:"reason,omitempty"`
}

func ValidateTierRun(p *Preparation, r *RunRequest) error {
	if p == nil || r == nil || p.Profile != "profiling" || r.Scenario != "trajectory" || r.PhaseBarriers || r.Operations != 1 || r.Samples < 1 || r.Samples > 100000 || r.Warmup < 0 || r.Warmup > 100000 {
		return fmt.Errorf("tier diagnostics require profiling trajectory, individual calls and bounded budgets without barriers")
	}
	w := p.Workload
	if w.ABI != "core" || w.Reset != "stateless" || w.Export == "" || w.Oracle.Kind != "exact_u64" || len(w.Oracle.Expected) != 1 || w.Oracle.Expected[0] > 0xffffffff || w.Oracle.Float != nil || w.Command != nil || w.Vectors != nil || w.Density != nil || w.Checkpoint != nil || w.GuestDensity != nil || w.Input != nil || len(w.Oracle.Memory) != 0 || w.Initialize != "" || w.HostProfile != "" {
		return fmt.Errorf("tier diagnostics require bare stateless exact-scalar core contract without initialization or memory/host fixtures")
	}
	for _, arg := range w.Args {
		if arg > 0xffffffff {
			return fmt.Errorf("tier diagnostics require unsigned i32 argument bits")
		}
	}
	return nil
}

func (t TierWindow) Validate(w Workload, s Sample) error {
	b, err := hex.DecodeString(w.SHA256)
	if err != nil || len(b) != 32 || t.ModuleSHA256 != w.SHA256 || t.Export != w.Export || (t.Version != 1 && t.Version != 2) || t.Collector != "V8/testing-code-tier-intrinsics" || t.CollectorVersion == "" || t.Scope != "exported_entry_code_nonatomic_boundary_snapshots" || t.Quality != "engine_reported" || s.Index < 0 || t.Invocation != s.Index+1 || s.Operations != 1 || s.SampleType != "individual_operation" {
		return fmt.Errorf("invalid tier diagnostic identity or scope")
	}
	if t.Version == 1 {
		if t.InvocationOutcome != "" || t.FailureReason != "" {
			return fmt.Errorf("legacy tier reading cannot declare invocation outcome")
		}
	} else {
		switch t.InvocationOutcome {
		case "returned":
			if !s.Verified || t.FailureReason != "" {
				return fmt.Errorf("returned tier call is not verified")
			}
		case "oracle_mismatch":
			if s.Verified || t.FailureReason == "" || len(s.Result) != 1 || s.Result[0] > 0xffffffff {
				return fmt.Errorf("invalid tier oracle failure")
			}
		case "guest_trap":
			if s.Verified || t.FailureReason == "" || len(s.Result) != 0 {
				return fmt.Errorf("invalid tier guest failure")
			}
		default:
			return fmt.Errorf("unknown tier invocation outcome")
		}
	}
	for _, r := range []TierReading{t.Before, t.After} {
		if r.StartNS < 0 || r.EndNS < r.StartNS {
			return fmt.Errorf("invalid tier reading clock")
		}
		switch r.State {
		case "uncompiled", "liftoff", "optimizing":
			if r.Reason != "" {
				return fmt.Errorf("available tier reading has a reason")
			}
		case "unavailable":
			if r.Reason == "" {
				return fmt.Errorf("unavailable tier reading lacks reason")
			}
		default:
			return fmt.Errorf("unknown code tier classification")
		}
	}
	if t.Before.EndNS > t.OperationStartNS || t.OperationEndNS < t.OperationStartNS || t.OperationEndNS > t.After.StartNS || t.OperationEndNS-t.OperationStartNS != s.ElapsedNS {
		return fmt.Errorf("tier snapshots overlap operation or differ from elapsed time")
	}
	return nil
}
