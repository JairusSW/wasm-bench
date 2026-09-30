package main

import (
	"github.com/tetratelabs/wazero/api"
	"reflect"
)

// wazero 1.12.0 can return an api.Memory containing a nil *MemoryInstance
// for modules without memory. Do not call methods on that typed-nil value.
func hasMemory(m api.Module) bool {
	if m == nil {
		return false
	}
	memory := m.Memory()
	if memory == nil {
		return false
	}
	v := reflect.ValueOf(memory)
	return v.Kind() != reflect.Pointer || !v.IsNil()
}
