(module
  (import "env" "function" (func (param i32) (result i32)))
  (import "env" "memory" (memory 1 2))
  (import "env" "table" (table 1 2 funcref))
  (import "env" "global" (global i32))
  (func (export "benchmark") (param f32 i64) (result f64)
    local.get 0 f64.promote_f32)
)
