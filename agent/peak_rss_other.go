//go:build !darwin && !linux

package agent

import "os"

func processPeakRSS(_ *os.ProcessState) (float64, bool) { return 0, false }
