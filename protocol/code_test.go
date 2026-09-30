package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

func codeFixture() CodeImage {
	data := []byte{0, 1, 2, 255}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	return CodeImage{Version: 1, ModuleSHA256: digest, SHA256: digest, Architecture: "arm64", Backend: "railshot", Format: "raw-native-image", SectionKind: "mixed_code_and_embedded_data", Event: "compiled_snapshot", FunctionAttribution: "unavailable", Data: data}
}

func TestCodeImageRoundTrip(t *testing.T) {
	c := codeFixture()
	encoded, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var got CodeImage
	if err = json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if err = got.Validate(c.ModuleSHA256); err != nil {
		t.Fatal(err)
	}
	if string(got.Data) != string(c.Data) {
		t.Fatal("binary bytes changed")
	}
}

func TestCodeImageRejectsInvalidEvidence(t *testing.T) {
	for _, mode := range []string{"digest", "module", "version", "arch", "format", "section", "event", "attribution", "backend", "size"} {
		t.Run(mode, func(t *testing.T) {
			c := codeFixture()
			module := c.ModuleSHA256
			switch mode {
			case "digest":
				c.Data[0] = 3
			case "module":
				module = "bad"
			case "version":
				c.Version = 2
			case "arch":
				c.Architecture = "unknown"
			case "format":
				c.Format = "serialized-artifact"
			case "section":
				c.SectionKind = "guest_instructions"
			case "event":
				c.Event = "creation"
			case "attribution":
				c.FunctionAttribution = "inferred"
			case "backend":
				c.Backend = ""
			case "size":
				c.Data = make([]byte, MaxCodeImageBytes+1)
			}
			if c.Validate(module) == nil {
				t.Fatal("invalid image accepted")
			}
		})
	}
}

func TestAttributedCodeRanges(t *testing.T) {
	fixture := func() CodeImage {
		c := codeFixture()
		c.Version = 2
		c.Backend = "cranelift"
		c.FunctionAttribution = "engine_reported"
		c.Functions = []CodeFunction{{WasmIndex: 1, Offset: 0, Length: 2, Tier: "cranelift"}, {WasmIndex: 2, Offset: 2, Length: 2, Tier: "cranelift"}}
		return c
	}
	c := fixture()
	if err := c.Validate(c.ModuleSHA256); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"overflow", "bounds", "overlap", "duplicate", "module", "tier", "generation", "empty", "legacy"} {
		t.Run(mode, func(t *testing.T) {
			c := fixture()
			switch mode {
			case "overflow":
				c.Functions[0].Length = ^uint64(0)
			case "bounds":
				c.Functions[0].Offset = 5
			case "overlap":
				c.Functions[1].Offset = 1
			case "duplicate":
				c.Functions[1].WasmIndex = 1
			case "module":
				c.Functions[0].ModuleIndex = 1
			case "tier":
				c.Functions[0].Tier = "unknown"
			case "generation":
				c.Functions[0].Generation = 1
			case "empty":
				c.Functions[0].Length = 0
			case "legacy":
				c.Version = 1
				c.FunctionAttribution = "unavailable"
			}
			if c.Validate(c.ModuleSHA256) == nil {
				t.Fatal("invalid range accepted")
			}
		})
	}
}
