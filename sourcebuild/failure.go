package sourcebuild

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
)

// Failed results are partial diagnostic receipts, never successful build
// measurements. The enclosing benchmark seal protects their logs and snapshots.
func verifyFailedBuild(root, bundle string, r Result, l Lock, c BenchmarkConfig, status string) error {
	if r.Schema != 1 || r.Kind != "source_build" || !reflect.DeepEqual(r.Lock, l) || r.CollectionProfile != c.Profile || !reflect.DeepEqual(r.ResourcePolicy, c.Resources) || len(r.Steps) > len(l.Recipe.Steps) {
		return fmt.Errorf("failed source build identity mismatch")
	}
	if status != buildFailureStatus(r) {
		return fmt.Errorf("failed source build classification differs from evidence")
	}
	for i, step := range r.Steps {
		if step.ContextError != "" && (i != len(r.Steps)-1 || (step.ContextError != "deadline_exceeded" && step.ContextError != "canceled")) {
			return fmt.Errorf("invalid failed tool context observation")
		}
		if step.Tool != l.Recipe.Steps[i].Tool || step.ElapsedNS < 0 || step.Log != fmt.Sprintf("step-%03d.log", i) {
			return fmt.Errorf("invalid failed build step")
		}
		info, err := os.Stat(filepath.Join(root, bundle, step.Log))
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("missing failed build log: %s", step.Log)
		}
		if c.Profile == "cpu" {
			if err := step.CPU.validate(); err != nil {
				return err
			}
		} else if step.CPU != nil {
			return fmt.Errorf("unexpected failed build CPU instrumentation")
		}
		// Only the final attempted step may contain OOM/cleanup failure or a
		// setup failure before spawn. Validate all other fields unchanged.
		if step.Resources != nil && i == len(r.Steps)-1 {
			evidence := *step.Resources
			if evidence.Isolation == nil {
				if c.Resources == nil || evidence.WallNS != 0 || step.ElapsedNS != 0 || evidence.OOM || evidence.CleanupError != "" || len(evidence.Observations) != 0 {
					return fmt.Errorf("invalid pre-spawn failure evidence")
				}
				continue
			}
			evidence.OOM, evidence.CleanupError = false, ""
			step.Resources = &evidence
		}
		if err := validateStepResources(step, c.Resources, c.Profile); err != nil {
			return err
		}
	}
	return nil
}
