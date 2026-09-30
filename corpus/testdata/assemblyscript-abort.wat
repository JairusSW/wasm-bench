;; SPDX-License-Identifier: MIT
(module
  (import "env" "abort" (func $abort (param i32 i32 i32 i32)))
  (memory (export "memory") 1)
  (func (export "initialize"))
  (func $fail (export "abort_init")
    ;; Deliberately invalid pointers must still cause a bounded host failure.
    (call $abort (i32.const -1) (i32.const -2147483648) (i32.const 7) (i32.const 11)))
  (func (export "benchmark") (result i32) (i32.const 7))
  (func (export "abort_benchmark") (result i32)
    (call $fail)
    ;; A no-op abort stub would incorrectly pass the ordinary result oracle.
    (i32.const 7))
)
