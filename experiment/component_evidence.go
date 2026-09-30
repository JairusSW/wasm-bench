package experiment

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
)

func validateComponentTree(data []byte, digest string) error {
	var report struct {
		Bytes uint64 `json:"bytes"`
		Nodes []struct {
			ID       int    `json:"id"`
			Parent   *int   `json:"parent"`
			Encoding string `json:"encoding"`
			Start    uint64 `json:"byte_start"`
			End      uint64 `json:"byte_end"`
			Bytes    uint64 `json:"bytes"`
			SHA      string `json:"sha256"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return err
	}
	if report.Bytes < 8 || len(report.Nodes) == 0 {
		return fmt.Errorf("missing component hierarchy")
	}
	lastChildEnd := map[int]uint64{}
	for i, n := range report.Nodes {
		h, err := hex.DecodeString(n.SHA)
		if err != nil || len(h) != 32 || n.ID != i || n.End < n.Start || n.End-n.Start != n.Bytes || n.Bytes < 8 || n.End > report.Bytes || (n.Encoding != "component" && n.Encoding != "core-module") {
			return fmt.Errorf("invalid component node identity or range")
		}
		if i == 0 {
			if n.Parent != nil || n.Encoding != "component" || n.Start != 0 || n.End != report.Bytes || n.SHA != digest {
				return fmt.Errorf("invalid component root")
			}
		} else {
			if n.Parent == nil || *n.Parent < 0 || *n.Parent >= i {
				return fmt.Errorf("invalid component parent")
			}
			p := report.Nodes[*n.Parent]
			if p.Encoding != "component" || n.Start < p.Start+8 || n.End > p.End || n.Start < lastChildEnd[*n.Parent] {
				return fmt.Errorf("invalid nested component range")
			}
			lastChildEnd[*n.Parent] = n.End
		}
	}
	return nil
}

func validateArtifactABI(data []byte, abi string) error {
	var report struct {
		Encoding string `json:"encoding"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return err
	}
	if (report.Encoding == "component") != (abi == "component") {
		return fmt.Errorf("workload ABI %q does not match artifact encoding %q", abi, report.Encoding)
	}
	return nil
}
