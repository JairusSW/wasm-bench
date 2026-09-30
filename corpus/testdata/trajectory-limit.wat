(module
  ;; Adversarial test fixture, not a stateless corpus workload. Exactly five
  ;; calls succeed: catches both hidden pre-calls and per-sample reinstantiation.
  (global $calls (mut i32) (i32.const 0))
  (func (export "run") (result i32)
    global.get $calls i32.const 1 i32.add global.set $calls
    global.get $calls i32.const 5 i32.gt_u if unreachable end
    i32.const 7))
