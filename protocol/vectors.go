package protocol

import (
	"bytes"
	"encoding/hex"
	"fmt"
)

// VectorContract describes an ordered sequence on one initialized instance.
// Each call receives (input address, input byte length, output address).
// Pointer exports override, rather than add to, their corresponding offsets.
type VectorContract struct {
	InputOffset         uint32       `json:"input_offset"`
	OutputOffset        uint32       `json:"output_offset"`
	InputPointerExport  string       `json:"input_ptr_export,omitempty"`
	OutputPointerExport string       `json:"output_ptr_export,omitempty"`
	OutputLen           int          `json:"output_len"`
	Mod                 int          `json:"mod,omitempty"`
	Cases               []VectorCase `json:"cases"`
}

type VectorCase struct {
	Len int    `json:"len"`
	Out string `json:"out"`
}

type preparedVector struct{ input, expected []byte }

// PreparedVectors owns generated inputs and decoded exact oracles. Preparation
// must occur outside measurement. It does not retain mutable caller slices.
type PreparedVectors struct {
	contract VectorContract
	cases    []preparedVector
}

// PrepareVectors enforces an explicit total input+oracle byte budget before
// allocating. A caller must record its admission budget; zero is not unlimited.
func PrepareVectors(v VectorContract, maxBytes uint64) (*PreparedVectors, error) {
	if len(v.Cases) == 0 || v.OutputLen <= 0 || v.Mod < 0 {
		return nil, fmt.Errorf("invalid vector dimensions")
	}
	var total uint64
	for i, c := range v.Cases {
		if c.Len < 0 || uint64(c.Len) > 0xffffffff || uint64(v.OutputLen) > 0xffffffff {
			return nil, fmt.Errorf("vector %d exceeds wasm32 dimensions", i)
		}
		if len(c.Out)%2 != 0 || len(c.Out)/2 != v.OutputLen {
			return nil, fmt.Errorf("vector %d output length mismatch", i)
		}
		size := uint64(c.Len) + uint64(v.OutputLen)
		if total > maxBytes || size > maxBytes-total {
			return nil, fmt.Errorf("vector input/oracle byte budget exceeded")
		}
		total += size
	}
	p := &PreparedVectors{contract: v, cases: make([]preparedVector, len(v.Cases))}
	p.contract.Cases = nil
	for i, c := range v.Cases {
		want, err := hex.DecodeString(c.Out)
		if err != nil {
			return nil, fmt.Errorf("vector %d output hex: %w", i, err)
		}
		input := make([]byte, c.Len)
		if v.Mod > 0 {
			for j := range input {
				input[j] = byte(j % v.Mod)
			}
		}
		p.cases[i] = preparedVector{input: input, expected: want}
	}
	return p, nil
}

// VectorInstance binds a single guest instance. Invoke may measure only the
// guest call; pointer resolution, writes, reads and equality checks are outside
// that callback. All callbacks must refer to the same initialized instance.
type VectorInstance struct {
	ReleaseBarrier func(int, string) error
	Snapshot       func() []Observation
	Pointer        func(string) ([]uint64, error)
	Write          func(uint32, []byte) bool
	Invoke         func(input, length, output uint32) error
	Read           func(uint32, uint32) ([]byte, bool)
}

// Run checks every output immediately before the next case can overwrite it.
// It neither resets the instance between cases nor changes case ordering.
func (p *PreparedVectors) Run(instance VectorInstance) error {
	if p == nil || len(p.cases) == 0 || instance.Write == nil || instance.Invoke == nil || instance.Read == nil {
		return fmt.Errorf("missing vector executor")
	}
	resolve := func(name string, offset uint32) (uint32, error) {
		if name == "" {
			return offset, nil
		}
		if instance.Pointer == nil {
			return 0, fmt.Errorf("missing vector pointer resolver")
		}
		v, err := instance.Pointer(name)
		if err != nil {
			return 0, err
		}
		if len(v) != 1 || v[0] > 0xffffffff {
			return 0, fmt.Errorf("invalid wasm32 vector pointer")
		}
		return uint32(v[0]), nil
	}
	in, err := resolve(p.contract.InputPointerExport, p.contract.InputOffset)
	if err != nil {
		return fmt.Errorf("vector input pointer: %w", err)
	}
	out, err := resolve(p.contract.OutputPointerExport, p.contract.OutputOffset)
	if err != nil {
		return fmt.Errorf("vector output pointer: %w", err)
	}
	for i, c := range p.cases {
		if uint64(in)+uint64(len(c.input)) > 1<<32 || uint64(out)+uint64(len(c.expected)) > 1<<32 {
			return fmt.Errorf("vector %d address overflow", i)
		}
		if !instance.Write(in, c.input) {
			return fmt.Errorf("vector %d input outside memory", i)
		}
		if err := instance.Invoke(in, uint32(len(c.input)), out); err != nil {
			return fmt.Errorf("vector %d invocation: %w", i, err)
		}
		got, ok := instance.Read(out, uint32(len(c.expected)))
		if !ok || !bytes.Equal(got, c.expected) {
			return fmt.Errorf("incorrect result: vector %d memory mismatch", i)
		}
	}
	return nil
}
