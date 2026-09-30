package main

import (
	"fmt"
	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/protocol"
)

func (a *adapter) runProfile(r *protocol.RunRequest) (samples []protocol.Sample, artifact *protocol.CPUProfile, err error) {
	if err = protocol.ValidateProfilingRun(a.prep, r); err != nil {
		return
	}
	if !a.interpreter {
		return nil, nil, fmt.Errorf("Go CPU profiling is enabled only for the interpreter; native guest stack unwinding is not qualified")
	}
	p := collectors.StartGoCPUProfile(a.prep.ArtifactSHA256)
	defer func() { result := p.Stop(); artifact = &result }()
	samples, err = a.run(r)
	return
}
