(module
  (import "env" "exit" (func $exit (param i32)))
  (memory (export "memory") 1)
  (global $sp (mut i32) (i32.const 1024))
  (func (export "__wasm_call_ctors"))
  (func (export "stackAlloc") (param $size i32) (result i32)
    (local $old i32)
    global.get $sp
    local.tee $old
    local.get $size
    i32.add
    global.set $sp
    local.get $old)
  (func (export "main") (param $argc i32) (param $argv i32) (result i32)
    local.get $argc
    i32.const 1
    i32.eq
    if (result i32)
      local.get $argv
      i32.load
      i32.load8_u
      i32.const 102
      i32.eq
      if (result i32)
        i32.const 0
      else
        i32.const 3
      end
    else
      i32.const 4
    end
    call $exit
    unreachable))
