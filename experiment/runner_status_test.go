package experiment

import (
	"errors"
	"testing"

	"github.com/wasmbench/wasmbench/protocol"
)

func TestSetAdapterFailurePreservesDeclaredSupportOutcome(t *testing.T) {
	for _, status := range []string{"unsupported", "unavailable"} {
		trial := Trial{Status: "error"}
		resp := protocol.Response{Status: status, Reason: "adapter did not measure this scenario"}
		if !setAdapterFailure(&trial, resp, errors.New(status+": "+resp.Reason)) {
			t.Fatalf("%s adapter response was treated as an error", status)
		}
		if trial.Status != status || trial.Reason != resp.Reason {
			t.Fatalf("adapter outcome changed: %+v", trial)
		}
	}
}

func TestSetAdapterFailureKeepsRealErrorsAndPartialSamplesAsFailures(t *testing.T) {
	for _, resp := range []protocol.Response{
		{Status: "error", Reason: "trap"},
		{Status: "unsupported", Reason: "unsupported after partial output", Samples: []protocol.Sample{{ElapsedNS: 1, Operations: 1}}},
	} {
		trial := Trial{Status: "error"}
		if setAdapterFailure(&trial, resp, errors.New("adapter failure")) {
			t.Fatalf("response should remain a failed trial: %+v", resp)
		}
	}
}
