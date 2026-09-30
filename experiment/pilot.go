package experiment

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
)

const PilotVersion = "launch-median-ci-budget-v1"

// The full decision is embedded in the confirmation lock, not a mutable path.
// This checks the local contract; the pilot-run command additionally re-derives
// the decision from checksum-verified source data. Neither certifies a host.
func validatePilotPlan(l Lock) error {
	if len(l.PilotPlan) == 0 {
		return nil
	}
	var p struct {
		Version         string  `json:"version"`
		Status          string  `json:"status"`
		Launches        int     `json:"chosen_launches"`
		Minimum         int     `json:"min_launches"`
		Maximum         int     `json:"max_launches"`
		Target          float64 `json:"target_relative_ci_half_width"`
		SourceLock      string  `json:"source_lock_sha256"`
		SourceChecksums string  `json:"source_checksums_sha256"`
	}
	if len(l.PilotPlan) > 16<<20 || json.Unmarshal(l.PilotPlan, &p) != nil {
		return fmt.Errorf("invalid embedded pilot plan")
	}
	if p.Version != PilotVersion || p.Status != "ready" || l.Options.Profile != "timing" || l.Options.Check || p.Launches != l.Options.Launches || p.Minimum < 6 || p.Maximum > 10000 || p.Minimum > p.Maximum || p.Launches < p.Minimum || p.Launches > p.Maximum || math.IsNaN(p.Target) || p.Target <= 0 || p.Target >= 1 {
		return fmt.Errorf("pilot plan must be ready and match the fixed timing launch budget")
	}
	for _, s := range []string{p.SourceLock, p.SourceChecksums} {
		digest, err := hex.DecodeString(s)
		if err != nil || len(digest) != 32 {
			return fmt.Errorf("pilot evidence digest required")
		}
	}
	return nil
}
