(module
  (memory (export "memory") 1)
  (global $calls (mut i32) (i32.const 0))
  (func (export "benchmark") (param $input i32) (param $length i32) (param $output i32)
    ;; Exactly two ordered calls per fresh instance: lengths zero, then seven.
    (if (i32.ge_u (global.get $calls) (i32.const 2)) (then unreachable))
    (if (i32.ne (local.get $length) (i32.mul (global.get $calls) (i32.const 7)))
      (then unreachable))
    (global.set $calls (i32.add (global.get $calls) (i32.const 1)))
    (i32.store8 (local.get $output) (i32.const 171)))
)
