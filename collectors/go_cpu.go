package collectors

import (
	"crypto/sha256"
	"encoding/hex"
	"runtime"
	"runtime/pprof"

	"github.com/wasmbench/wasmbench/protocol"
)

type profileBuffer struct {
	data     []byte
	limit    int
	overflow bool
}

func (b *profileBuffer) Write(p []byte) (int, error) {
	if b.overflow || len(p) > b.limit-len(b.data) {
		b.overflow = true
		return len(p), nil
	}
	b.data = append(b.data, p...)
	return len(p), nil
}

// GoCPUProfile owns only a successfully started profiler. Stop waits for the
// runtime's writer before reading the buffer; no buffer access races its writes.
type GoCPUProfile struct {
	buffer          profileBuffer
	artifact        protocol.CPUProfile
	active, stopped bool
}

func StartGoCPUProfile(module string) *GoCPUProfile {
	p := &GoCPUProfile{buffer: profileBuffer{limit: protocol.MaxProfileBytes}, artifact: protocol.CPUProfile{Version: 1, ModuleSHA256: module, Format: "pprof-gzip", Collector: "runtime/pprof", CollectorVersion: runtime.Version(), Scope: "adapter_process_go_cpu", Window: "run_request_including_setup_warmup_verification", Quality: "sampled", Status: "unavailable"}}
	if err := pprof.StartCPUProfile(&p.buffer); err != nil {
		p.artifact.Reason = err.Error()
	} else {
		p.active = true
	}
	return p
}
func (p *GoCPUProfile) Stop() protocol.CPUProfile {
	if p.stopped {
		return p.artifact
	}
	p.stopped = true
	if !p.active {
		return p.artifact
	}
	pprof.StopCPUProfile()
	p.active = false
	if p.buffer.overflow {
		p.artifact.Reason = "CPU profile exceeded 16 MiB budget; no partial artifact retained"
		p.buffer.data = nil
		return p.artifact
	}
	p.artifact.Status = "collected"
	p.artifact.Data = p.buffer.data
	sum := sha256.Sum256(p.artifact.Data)
	p.artifact.SHA256 = hex.EncodeToString(sum[:])
	return p.artifact
}
