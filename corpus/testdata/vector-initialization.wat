(module
  (memory (export "memory") 1)
  (global $calls (mut i32) (i32.const -3))
  (func $start
    (if (i32.ne (global.get $calls) (i32.const -3)) (then unreachable))
    (global.set $calls (i32.const -2)))
  (start $start)
  (func (export "initialize")
    ;; Start must have run exactly once before explicit application init.
    (if (i32.ne (global.get $calls) (i32.const -2)) (then unreachable))
    (global.set $calls (i32.const 0)))
  (func (export "benchmark") (param $input i32) (param $length i32) (param $output i32)
    ;; Each fresh initialized instance must receive exactly this ordered pair.
    (if (i32.ge_u (global.get $calls) (i32.const 2)) (then unreachable))
    (if (i32.ne (local.get $length) (i32.mul (global.get $calls) (i32.const 7)))
      (then unreachable))
    (global.set $calls (i32.add (global.get $calls) (i32.const 1)))
    (i32.store8 (local.get $output) (i32.const 171)))
)
