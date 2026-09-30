package protocol

import (
	"bufio"
	"encoding/json"
	"fmt"
)

// Barrier blocks the adapter until the collector acknowledges a boundary.
func Barrier(scanner *bufio.Scanner, encoder *json.Encoder, requestID int, event PhaseEvent) error {
	if err := encoder.Encode(Response{Version: Version, ID: requestID, Status: "phase", Phase: &event}); err != nil {
		return err
	}
	if !scanner.Scan() {
		return fmt.Errorf("phase acknowledgement missing")
	}
	var ack Request
	if err := json.Unmarshal(scanner.Bytes(), &ack); err != nil {
		return err
	}
	if ack.Version != Version || ack.ID != requestID || ack.Method != "continue" {
		return fmt.Errorf("invalid phase acknowledgement")
	}
	return nil
}
