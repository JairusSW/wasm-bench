;; SPDX-License-Identifier: MIT
;; A JavaScript prototype method must not satisfy an undeclared host import.
(module
  (import "env" "valueOf" (func (result i32)))
  (func (export "initialize"))
  (func (export "benchmark") (result i32) (i32.const 7))
)
