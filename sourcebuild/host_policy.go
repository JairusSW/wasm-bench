package sourcebuild

import "github.com/wasmbench/wasmbench/experiment"

// Source benchmarks use the same boundary contract as runtime experiments.
// Trial execution status is retained independently of measurement eligibility.
func (b BuildBenchmark) hostEvidence() experiment.Manifest {
	options := experiment.Options{}
	if b.Config.Resources != nil {
		options.Resources = *b.Config.Resources
	}
	return experiment.Manifest{
		Lock:             experiment.Lock{RequireIRQAffinity: b.Config.RequireIRQAffinity, HostPolicy: b.Config.HostPolicy, RequireIsolatedCPUPartition: b.Config.RequireIsolatedCPUPartition, Options: options},
		IRQAffinityStart: b.IRQAffinityStart, IRQAffinityEnd: b.IRQAffinityEnd,
		CPUPartitionStart: b.CPUPartitionStart, CPUPartitionEnd: b.CPUPartitionEnd,
		Host: b.Host, HostEnd: b.HostEnd, HostStartCheck: b.HostStartCheck,
		HostEndCheck: b.HostEndCheck, Publication: b.Publication,
	}
}

func (b BuildBenchmark) ValidateHostEvidence() error {
	return experiment.ValidateHostEvidence(b.hostEvidence())
}

func (b BuildBenchmark) HostBaselineAllowsMeasurements() bool {
	return experiment.HostBaselineAllowsMeasurements(b.hostEvidence())
}
