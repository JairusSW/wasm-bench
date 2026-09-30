package agent

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
)

const HostPolicyVersion = "observed-host-baseline-v1"

// HostPolicy pins observed identity/settings, including their availability.
// Matching missing facts or uncontrolled settings does not certify control.
type HostPolicy struct {
	Version  string `json:"version"`
	Expected Host   `json:"expected"`
}
type HostPolicyCheck struct {
	Version string   `json:"version"`
	Stage   string   `json:"stage"`
	Status  string   `json:"status"`
	Changed []string `json:"changed_fields"`
}

func (p *HostPolicy) Validate() error {
	if p == nil {
		return nil
	}
	h := p.Expected
	if p.Version != HostPolicyVersion || h.OS == "" || h.Arch == "" || h.Hostname == "" || h.PageSize <= 0 || h.CPUs <= 0 || h.Environment == nil || h.Policy == nil {
		return fmt.Errorf("invalid observed host baseline")
	}
	if h.Fingerprint != nil && h.Fingerprint.Version != HostFingerprintVersion {
		return fmt.Errorf("unsupported host fingerprint version")
	}
	data, err := json.Marshal(p)
	if err != nil || len(data) > 16<<20 {
		return fmt.Errorf("host baseline exceeds size budget")
	}
	return nil
}

func CheckHostPolicy(p *HostPolicy, h Host, stage string) HostPolicyCheck {
	c := HostPolicyCheck{Version: HostPolicyVersion, Stage: stage, Status: "matched_observed_baseline", Changed: []string{}}
	if p == nil {
		c.Status = "not_requested"
		return c
	}
	if p.Validate() != nil {
		c.Status = "invalid"
		return c
	}
	// JSON field-level comparison avoids depending on map order and provides
	// actionable top-level differences without hiding missing fingerprint facts.
	a, _ := json.Marshal(p.Expected)
	b, _ := json.Marshal(h)
	var expected, actual map[string]json.RawMessage
	json.Unmarshal(a, &expected)
	json.Unmarshal(b, &actual)
	keys := map[string]bool{}
	for k := range expected {
		keys[k] = true
	}
	for k := range actual {
		keys[k] = true
	}
	for k := range keys {
		if !reflect.DeepEqual(expected[k], actual[k]) {
			c.Changed = append(c.Changed, k)
		}
	}
	sort.Strings(c.Changed)
	if len(c.Changed) > 0 {
		c.Status = "mismatch"
	}
	return c
}

func (c HostPolicyCheck) Err() error {
	if c.Status == "matched_observed_baseline" || c.Status == "not_requested" {
		return nil
	}
	return fmt.Errorf("host baseline %s at %s: %v", c.Status, c.Stage, c.Changed)
}
