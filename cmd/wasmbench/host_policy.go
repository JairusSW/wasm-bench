package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/experiment"
)

func captureHostPolicy(args []string) error {
	f := flags("host-policy")
	out := f.String("out", "", "new baseline JSON file (stdout if omitted)")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("host-policy accepts no positional arguments")
	}
	p := agent.HostPolicy{Version: agent.HostPolicyVersion, Expected: agent.IdentifyHost()}
	if err := p.Validate(); err != nil {
		return err
	}
	if *out != "" {
		return experiment.WriteJSON(*out, p)
	}
	return output(p)
}

func loadHostPolicy(path string) (*agent.HostPolicy, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > 16<<20 {
		return nil, fmt.Errorf("host baseline exceeds 16 MiB")
	}
	decoder := json.NewDecoder(io.LimitReader(file, (16<<20)+1))
	decoder.DisallowUnknownFields()
	var p agent.HostPolicy
	if err := decoder.Decode(&p); err != nil {
		return nil, err
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, fmt.Errorf("host baseline must contain one JSON value")
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}
