//go:build linux

package agent

import "os"

func probeIRQAffinityLive(p IRQAffinityProbe) IRQAffinityProbe {
	return readIRQAffinity(os.DirFS("/"), p)
}
