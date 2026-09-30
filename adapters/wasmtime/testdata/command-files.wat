(module
  (import "wasi_snapshot_preview1" "path_open"
    (func $open (param i32 i32 i32 i32 i32 i64 i64 i32 i32) (result i32)))
  (import "wasi_snapshot_preview1" "fd_read"
    (func $read (param i32 i32 i32 i32) (result i32)))
  (import "wasi_snapshot_preview1" "fd_write"
    (func $write (param i32 i32 i32 i32) (result i32)))
  (import "wasi_snapshot_preview1" "random_get"
    (func $random (param i32 i32) (result i32)))
  (import "wasi_snapshot_preview1" "clock_time_get"
    (func $clock (param i32 i64 i32) (result i32)))
  (memory (export "memory") 1)
  (data (i32.const 100) "input")
  (data (i32.const 110) "../input")
  (func $ok (param $errno i32)
    (if (local.get $errno) (then unreachable)))
  (func (export "_start") (local $fd i32)
    ;; Read-only preopen at fd 3; open and read the pinned fixture.
    (call $ok (call $open (i32.const 3) (i32.const 0) (i32.const 100)
      (i32.const 5) (i32.const 0) (i64.const 2) (i64.const 0) (i32.const 0) (i32.const 0)))
    (local.set $fd (i32.load (i32.const 0)))
    (i32.store (i32.const 8) (i32.const 200))
    (i32.store (i32.const 12) (i32.const 3))
    (call $ok (call $read (local.get $fd) (i32.const 8) (i32.const 1) (i32.const 16)))
    (if (i32.ne (i32.load (i32.const 16)) (i32.const 3)) (then unreachable))
    ;; Asking for write rights must fail, even for an existing file.
    (if (i32.eqz (call $open (i32.const 3) (i32.const 0) (i32.const 100)
      (i32.const 5) (i32.const 0) (i64.const 64) (i64.const 0) (i32.const 0) (i32.const 0)))
      (then unreachable))
    ;; Parent traversal cannot escape the preopened fixture root.
    (if (i32.eqz (call $open (i32.const 3) (i32.const 0) (i32.const 110)
      (i32.const 8) (i32.const 0) (i64.const 2) (i64.const 0) (i32.const 0) (i32.const 0)))
      (then unreachable))
    (i64.store (i32.const 32) (i64.const -1))
    (call $ok (call $random (i32.const 32) (i32.const 8)))
    (if (i64.ne (i64.load (i32.const 32)) (i64.const 0)) (then unreachable))
    (call $ok (call $clock (i32.const 0) (i64.const 0) (i32.const 32)))
    (if (i64.ne (i64.load (i32.const 32)) (i64.const 1640995200000000000)) (then unreachable))
    (call $ok (call $clock (i32.const 0) (i64.const 0) (i32.const 32)))
    (if (i64.ne (i64.load (i32.const 32)) (i64.const 1640995200001000000)) (then unreachable))
    (call $ok (call $write (i32.const 1) (i32.const 8) (i32.const 1) (i32.const 16)))
  )
)
