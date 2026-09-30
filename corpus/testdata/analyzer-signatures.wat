(module
  (type $calc (func (param i32 i64) (result f64 i32)))
  (import "env" "function" (func (param i64) (result i32)))
  (import "env" "memory" (memory 1))
  (import "env" "table" (table 1 funcref))
  (import "env" "global" (global i32))
  (func (export "calculate") (type $calc) (local i64)
    f64.const 2
    i32.const 7)
)
