(module
  ;; Adversarial fixture, NOT a stateless corpus workload. Exactly five calls
  ;; pass the oracle. Hidden pre-calls and per-sample resets both fail the test.
  (global $calls (mut i32) (i32.const 0))
  (func (export "run") (result f64)
    global.get $calls i32.const 1 i32.add global.set $calls
    global.get $calls i32.const 5 i32.gt_u
    if (result f64) f64.const 8 else f64.const 7 end))
