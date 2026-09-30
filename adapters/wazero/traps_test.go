package main

import (
	"errors"
	"fmt"
	"testing"
)

func TestTrapClassifierRejectsUntypedMessages(t *testing.T) {
	for _, err := range []error{nil, errors.New("unreachable"), errors.New("wasm error: unreachable"), fmt.Errorf("wrap: %w", errors.New("integer divide by zero"))} {
		if classifyTrap(err) != nil {
			t.Fatal("classified arbitrary error", err)
		}
	}
}
