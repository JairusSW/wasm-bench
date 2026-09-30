(module
  (memory (export "memory") 1)
  ;; Reusing an instance makes later calls return normally, exposing reset bugs.
  (global $used (mut i32) (i32.const 0))
  (func $already_used (result i32)
    global.get $used i32.const 1 global.set $used)
  (func (export "unreachable") call $already_used if return end unreachable)
  (func (export "memory_out_of_bounds") call $already_used if return end i32.const 65536 i32.load drop)
  (func (export "integer_divide_by_zero") call $already_used if return end i32.const 7 i32.const 0 i32.div_s drop)
  (func (export "integer_overflow") call $already_used if return end i32.const -2147483648 i32.const -1 i32.div_s drop)
  (func (export "no_trap"))
)
