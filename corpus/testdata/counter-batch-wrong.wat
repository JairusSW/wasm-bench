;; SPDX-License-Identifier: MIT
;; Adversarial test: an incorrect middle result cannot hide behind the last result.
(module
  (global $calls (mut i32) (i32.const 0))
  (func (export "run") (result i32)
    global.get $calls i32.const 1 i32.add global.set $calls
    global.get $calls i32.const 3 i32.eq
    if (result i32) i32.const 8 else i32.const 7 end))
