;; MIT; host-import-free Component Model call/reset qualification fixture.
(component
  (core module $m
    (global $calls (mut i64) (i64.const 0))
    (func (export "benchmark") (param i64) (result i64)
      local.get 0 i64.const 1 i64.add global.get $calls i64.add
      global.get $calls i64.const 1 i64.add global.set $calls)
    (func (export "wrong-type") (param i32) (result i32) local.get 0)
    (func (export "trap") (param i64) (result i64) unreachable))
  (core instance $i (instantiate $m))
  (func (export "benchmark") (param "value" u64) (result u64)
    (canon lift (core func $i "benchmark")))
  (func (export "wrong-type") (param "value" u32) (result u32)
    (canon lift (core func $i "wrong-type")))
  (func (export "trap") (param "value" u64) (result u64)
    (canon lift (core func $i "trap"))))
