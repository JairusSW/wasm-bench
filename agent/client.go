package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/wasmbench/wasmbench/protocol"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

type Client struct {
	cmd             *exec.Cmd
	in              io.WriteCloser
	out             *bufio.Scanner
	cancel          context.CancelFunc
	id              int
	log             *os.File
	once            sync.Once
	cgroup          *cgroup
	closeErr        error
	preparedProfile string
}

func Start(parent context.Context, command []string, logPath string, timeout time.Duration) (*Client, error) {
	return StartIsolated(parent, command, logPath, timeout, ResourcePolicy{})
}

func StartIsolated(parent context.Context, command []string, logPath string, timeout time.Duration, policy ResourcePolicy) (*Client, error) {
	if len(command) == 0 {
		return nil, fmt.Errorf("empty adapter command")
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	g, err := prepareCgroup(cmd, policy)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("resource isolation: %w", err)
	}
	started := false
	defer func() {
		if !started {
			g.close()
		}
	}()
	if g != nil {
		cmd.Cancel = func() error {
			if e := g.kill(); e != nil {
				return cmd.Process.Kill()
			}
			return nil
		}
	}
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		cancel()
		return nil, err
	}
	cmd.Stderr = log
	in, err := cmd.StdinPipe()
	if err != nil {
		log.Close()
		cancel()
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		in.Close()
		log.Close()
		cancel()
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		in.Close()
		log.Close()
		cancel()
		return nil, err
	}
	s := bufio.NewScanner(out)
	s.Buffer(make([]byte, 4096), 64<<20)
	started = true
	return &Client{cmd: cmd, in: in, out: s, cancel: cancel, log: log, cgroup: g}, nil
}
func (c *Client) PID() int { return c.cmd.Process.Pid }
func (c *Client) Call(req protocol.Request) (protocol.Response, error) {
	return c.CallPhased(req, nil)
}
func (c *Client) CallPhased(req protocol.Request, onPhase func(protocol.PhaseEvent) error) (protocol.Response, error) {
	return c.call(req, onPhase, nil, nil)
}
func (c *Client) CallSnapshotInspection(req protocol.Request, onSnapshot func(protocol.SnapshotBoundary) error) (protocol.Response, error) {
	if c.preparedProfile != "memory" || req.Method != "inspect" || req.Run == nil || !req.Run.PhaseBarriers || onSnapshot == nil {
		return protocol.Response{}, fmt.Errorf("snapshot inspection requires prepared memory profile, inspect, barriers and observer")
	}
	return c.call(req, nil, onSnapshot, nil)
}
func (c *Client) CallSnapshotDensityInspection(req protocol.Request, observer func(protocol.SnapshotDensityBoundary) error) (protocol.Response, error) {
	if c.preparedProfile != "memory" || req.Method != "inspect" || req.Run == nil || req.Run.Scenario != protocol.SnapshotDensityScenario || !req.Run.PhaseBarriers || observer == nil {
		return protocol.Response{}, fmt.Errorf("density inspection requires prepared memory, inspect, barriers and observer")
	}
	return c.call(req, nil, nil, observer)
}
func (c *Client) call(req protocol.Request, onPhase func(protocol.PhaseEvent) error, onSnapshot func(protocol.SnapshotBoundary) error, onDensity func(protocol.SnapshotDensityBoundary) error) (protocol.Response, error) {
	if req.Method == "prepare" {
		c.preparedProfile = ""
	}
	c.id++
	req.Version = protocol.Version
	req.ID = c.id
	var resp protocol.Response
	if err := json.NewEncoder(c.in).Encode(req); err != nil {
		return resp, err
	}
	for {
		if !c.out.Scan() {
			if err := c.out.Err(); err != nil {
				return resp, err
			}
			return resp, fmt.Errorf("adapter terminated before response (see trial log)")
		}
		resp = protocol.Response{}
		if err := json.Unmarshal(c.out.Bytes(), &resp); err != nil {
			return resp, fmt.Errorf("invalid control response: %w", err)
		}
		if resp.Version != protocol.Version || resp.ID != req.ID {
			return resp, fmt.Errorf("protocol version or request identity mismatch")
		}
		if resp.Status == "snapshot_density_boundary" {
			if onDensity == nil || resp.SnapshotDensityBoundary == nil || resp.SnapshotDensityDiagnostics != nil || resp.SnapshotBoundary != nil || resp.SnapshotDiagnostics != nil || resp.Phase != nil || len(resp.Samples) != 0 || len(resp.Diagnostics) != 0 || resp.CodeImage != nil || resp.CPUProfile != nil || resp.CodeLifetime != nil || resp.EngineTrace != nil {
				return resp, fmt.Errorf("unsolicited/mixed density boundary")
			}
			if err := protocol.ValidateSnapshotDensityBoundary(*resp.SnapshotDensityBoundary); err != nil {
				return resp, err
			}
			if err := onDensity(*resp.SnapshotDensityBoundary); err != nil {
				return resp, err
			}
			if err := json.NewEncoder(c.in).Encode(protocol.Request{Version: protocol.Version, ID: req.ID, Method: "continue"}); err != nil {
				return resp, err
			}
			continue
		}
		if resp.SnapshotDensityBoundary != nil || (resp.SnapshotDensityDiagnostics != nil && (onDensity == nil || resp.Status != "ok" || resp.SnapshotBoundary != nil || resp.SnapshotDiagnostics != nil || resp.Phase != nil || len(resp.Samples) != 0 || len(resp.Diagnostics) != 0 || resp.CodeImage != nil || resp.CPUProfile != nil || resp.CodeLifetime != nil || resp.EngineTrace != nil)) {
			return resp, fmt.Errorf("density payload outside exclusive inspection")
		}
		if resp.Status == "snapshot_boundary" {
			if onSnapshot == nil || resp.SnapshotBoundary == nil || resp.SnapshotDiagnostics != nil || resp.Phase != nil || len(resp.Samples) != 0 {
				return resp, fmt.Errorf("unsolicited/mixed snapshot live boundary")
			}
			if err := protocol.ValidateSnapshotBoundary(*resp.SnapshotBoundary); err != nil {
				return resp, err
			}
			if err := onSnapshot(*resp.SnapshotBoundary); err != nil {
				return resp, err
			}
			if err := json.NewEncoder(c.in).Encode(protocol.Request{Version: protocol.Version, ID: req.ID, Method: "continue"}); err != nil {
				return resp, err
			}
			continue
		}
		if resp.SnapshotBoundary != nil {
			return resp, fmt.Errorf("snapshot boundary outside diagnostic handshake")
		}
		if resp.SnapshotDiagnostics != nil && (onSnapshot == nil || len(resp.Samples) != 0 || resp.Phase != nil) {
			return resp, fmt.Errorf("snapshot diagnostics outside inspection")
		}
		if resp.Status == "phase" {
			if onPhase == nil || resp.Phase == nil {
				return resp, fmt.Errorf("unsolicited phase barrier")
			}
			if err := onPhase(*resp.Phase); err != nil {
				return resp, err
			}
			if err := json.NewEncoder(c.in).Encode(protocol.Request{Version: protocol.Version, ID: req.ID, Method: "continue"}); err != nil {
				return resp, err
			}
			continue
		}
		if resp.Status != "ok" {
			return resp, fmt.Errorf("%s: %s", resp.Status, resp.Reason)
		}
		if req.Method == "prepare" && req.Prepare != nil {
			c.preparedProfile = req.Prepare.Profile
		}
		return resp, nil
	}
}
func (c *Client) Isolation() *Isolation { return c.cgroup.isolation() }

func (c *Client) VerifyResources() *ResourceVerification {
	return c.cgroup.verifyResources("response_end_before_cleanup")
}
func (c *Client) ResourceObservations() ([]protocol.Observation, bool) {
	return c.cgroup.observations()
}
func (c *Client) Close() error {
	c.once.Do(func() {
		c.in.Close()
		c.cancel()
		c.cmd.Wait()
		c.log.Close()
		c.cgroup.kill()
		c.closeErr = c.cgroup.close()
	})
	return c.closeErr
}

// PeakRSSObservation is valid only after Close has reaped the adapter process.
// It covers the adapter process lifetime, including startup and setup.
func (c *Client) PeakRSSObservation(scenario string) protocol.Observation {
	o := protocol.Observation{Metric: "process.peak_rss", DefinitionVersion: 1, Unit: "bytes", Scope: "adapter_process", Phase: scenario + "/process_lifetime", Collector: "wait4_rusage", CollectorVersion: "1", Quality: "kernel_accounted_peak", Profile: "memory", Status: "unavailable", Denominator: "process"}
	if value, ok := processPeakRSS(c.cmd.ProcessState); ok {
		o.Status = "available"
		o.Value = protocol.Value(value)
	} else {
		o.Reason = "child process peak RSS not available from wait4"
	}
	return o
}
