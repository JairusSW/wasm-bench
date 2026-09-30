package protocol

import "testing"

func TestCommandContract(t *testing.T) {
	for _, mode := range []string{"valid", "path", "digest", "budget", "collision", "oracle", "argv", "output", "mixed"} {
		t.Run(mode, func(t *testing.T) {
			c := &CommandContract{Argv: []string{"test"}, Files: map[string]CommandFile{"a": {Data: []byte("a"), SHA256: CommandDigest([]byte("a"))}}, StdoutSHA256: CommandDigest(nil), OutputLimit: 100}
			w := Workload{ABI: "wasi-command", Export: "_start", Reset: "fresh_instance_per_sample", HostProfile: "wasi-preview1-readonly-v1", Command: c, Oracle: Oracle{Kind: "exact_command"}}
			switch mode {
			case "path":
				c.Files["../escape"] = c.Files["a"]
			case "digest":
				c.Files["a"] = CommandFile{SHA256: CommandDigest(nil), Data: []byte("a")}
			case "budget":
				c.Stdin = make([]byte, CommandInputLimit+1)
			case "collision":
				c.Files["a/b"] = c.Files["a"]
			case "oracle":
				c.StdoutSHA256 = ""
			case "argv":
				c.Argv = []string{"a\x00b"}
			case "output":
				c.OutputLimit = 0
			case "mixed":
				w.Input = &MemoryInput{}
			}
			if err := ValidateCommand(w); (err == nil) != (mode == "valid") {
				t.Fatal(err)
			}
		})
	}
}

func TestCommandStdoutNormalizationRetainsRawAndOracleDigests(t *testing.T) {
	raw := []byte("block:       ; preds = %entry\n")
	normalized, err := NormalizeCommandStdout("llvm-ir-preds", raw)
	if err != nil || string(normalized) != "block: ; preds = %entry\n" {
		t.Fatalf("normalized stdout %q, %v", normalized, err)
	}
	contract := &CommandContract{Argv: []string{"clang"}, ExitCode: 0, StdoutSHA256: CommandDigest(normalized), StdoutNormalize: "llvm-ir-preds", StderrSHA256: CommandDigest(nil), OutputLimit: 1024}
	result := CommandResult{ExitCode: 0, StdoutSHA256: CommandDigest(raw), StdoutOracleSHA256: CommandDigest(normalized), StderrSHA256: CommandDigest(nil), StdoutBytes: uint64(len(raw))}
	if contract.StdoutSHA256 == result.StdoutSHA256 {
		t.Fatal("fixture should distinguish raw and normalized output")
	}
	if err := contract.Verify(result); err != nil {
		t.Fatal(err)
	}
	result.StdoutOracleSHA256 = ""
	if err := contract.Verify(result); err == nil {
		t.Fatal("accepted a normalized oracle without normalized digest evidence")
	}
	if _, err := NormalizeCommandStdout("future-normalizer", raw); err == nil {
		t.Fatal("accepted an unknown normalizer")
	}
}

func TestComponentCommandHostProfiles(t *testing.T) {
	for _, profile := range []string{"wasi-preview2-readonly-v1", "wasi-preview2-temporary-filesystem-v1"} {
		w := Workload{
			ABI: "component", Export: "_start", Reset: "fresh_instance_per_sample", HostProfile: profile,
			Command: &CommandContract{Argv: []string{"component"}, ExitCode: 0, StdoutSHA256: CommandDigest(nil), OutputLimit: 128},
			Oracle:  Oracle{Kind: "exact_command"},
		}
		if err := ValidateCommand(w); err != nil {
			t.Fatalf("profile %q: %v", profile, err)
		}
		w.Command.ExitCode = 2
		if err := ValidateCommand(w); err == nil {
			t.Fatalf("profile %q accepted unsupported Preview 2 exit code", profile)
		}
	}
}

func TestEmscriptenStdioCommandProfile(t *testing.T) {
	w := Workload{
		ABI: "emscripten", Export: "main", Reset: "fresh_instance_per_sample", HostProfile: EmscriptenStdioProfile,
		Command: &CommandContract{Argv: []string{"program"}, ExitCode: 0, StdoutSHA256: CommandDigest(nil), OutputLimit: 128},
		Oracle:  Oracle{Kind: "exact_command"},
	}
	if err := ValidateCommand(w); err != nil {
		t.Fatal(err)
	}
	w.HostProfile = "wasi-preview1-readonly-v1"
	if err := ValidateCommand(w); err == nil {
		t.Fatal("accepted an Emscripten command under the WASI host profile")
	}
}
