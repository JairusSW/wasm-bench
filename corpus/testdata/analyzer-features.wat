(module
  (memory 1)
  (func (export "benchmark") (result i32)
    i32.const 0
    i32.const 0
    i32.const 8
    memory.fill
    v128.const i32x4 0 0 0 0
    drop
    i32.const 7)
)
