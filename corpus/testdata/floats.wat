(module
  (func (export "sum_f64") (result f64)
    f64.const 0.1 f64.const 0.2 f64.add)
  (func (export "sqrt_f32") (result f32)
    f32.const 2 f32.sqrt)
  (func (export "negative_zero") (result f64) f64.const -0)
  (func (export "nan_f64") (result f64)
    f64.const -1 f64.sqrt)
  (func (export "infinity") (result f64) f64.const inf)
  (func (export "wrong_type") (result i64) i64.const 0)
  (func (export "with_args") (param f32 f64 i32 i64) (result f64)
    local.get 0 f64.promote_f32 local.get 1 f64.add
    local.get 2 f64.convert_i32_s f64.add
    local.get 3 f64.convert_i64_s f64.add)
  (func (export "mixed_results") (result f32 f64)
    f32.const 1.5 f64.const 2.25)
)
