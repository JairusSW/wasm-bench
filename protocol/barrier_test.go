package protocol

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestBarrierRequiresMatchingAcknowledgement(t *testing.T) {
	for _, ack := range []string{"", `{"version":1,"id":8,"method":"continue"}`, `{"version":1,"id":7,"method":"close"}`, `{"version":2,"id":7,"method":"continue"}`} {
		var out bytes.Buffer
		if err := Barrier(bufio.NewScanner(strings.NewReader(ack)), json.NewEncoder(&out), 7, PhaseEvent{Stage: "compiled"}); err == nil {
			t.Fatal("accepted bad acknowledgement", ack)
		}
	}
	var out bytes.Buffer
	if err := Barrier(bufio.NewScanner(strings.NewReader(`{"version":1,"id":7,"method":"continue"}`)), json.NewEncoder(&out), 7, PhaseEvent{Stage: "compiled"}); err != nil {
		t.Fatal(err)
	}
	var response Response
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Status != "phase" || response.ID != 7 || response.Phase.Stage != "compiled" {
		t.Fatal(response)
	}
}
