(module
  (func $answer (result i32) i32.const 7)
  (func (export "benchmark") (result i32) return_call $answer))
