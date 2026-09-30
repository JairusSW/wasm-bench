package protocol

import "fmt"

const AssemblyScriptAbortProfile = "assemblyscript-abort-v1"

// Never dereference guest pointers: aborts fail even with invalid pointers.
func AssemblyScriptAbort(message, file, line, column uint32) error {
	return fmt.Errorf("AssemblyScript abort: message_ptr=%d file_ptr=%d line=%d column=%d", message, file, line, column)
}
