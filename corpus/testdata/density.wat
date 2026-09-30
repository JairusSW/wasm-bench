;; MIT. Each fresh instance returns touched-byte checksum plus call count.
;; Reusing an instance for a second group member therefore fails its oracle.
(module
  (memory (export "memory") 1 1)
  (global $calls (mut i32) (i32.const 0))
  (func (export "benchmark") (param $bytes i32) (result i32)
    (local $i i32) (local $sum i32)
    (global.set $calls (i32.add (global.get $calls) (i32.const 1)))
    (block $done (loop $write
      (br_if $done (i32.ge_u (local.get $i) (local.get $bytes)))
      (i32.store8 (local.get $i) (i32.const 1))
      (local.set $i (i32.add (local.get $i) (i32.const 1)))
      (br $write)))
    (local.set $i (i32.const 0))
    (block $done (loop $read
      (br_if $done (i32.ge_u (local.get $i) (local.get $bytes)))
      (local.set $sum (i32.add (local.get $sum) (i32.load8_u (local.get $i))))
      (local.set $i (i32.add (local.get $i) (i32.const 1)))
      (br $read)))
    (i32.add (local.get $sum) (global.get $calls))))
