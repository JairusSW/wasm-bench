package protocol

import (
	"encoding/json"
	"math"
	"slices"
	"testing"
)

func TestValuesLossless(t *testing.T) {
	want := Values{0, 1<<53 + 1, math.MaxUint64}
	b, e := json.Marshal(want)
	if e != nil {
		t.Fatal(e)
	}
	if string(b) != `["0","9007199254740993","18446744073709551615"]` {
		t.Fatalf("wire representation %s", b)
	}
	var got Values
	if e = json.Unmarshal(b, &got); e != nil {
		t.Fatal(e)
	}
	if !slices.Equal(want, got) {
		t.Fatal(got)
	}
	if e = json.Unmarshal([]byte(`[-1]`), &got); e == nil {
		t.Fatal("negative bit pattern accepted")
	}
	if e = json.Unmarshal([]byte(`[1.2]`), &got); e == nil {
		t.Fatal("fractional integer accepted")
	}
}
