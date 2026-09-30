use crate::{Result, Val, Value, anyhow, bail, bits};

fn decode(typ: &str, n: u64) -> Result<f64> {
    match typ {
        "f64" => Ok(f64::from_bits(n)),
        "f32" if n <= u32::MAX as u64 => Ok(f32::from_bits(n as u32) as f64),
        _ => bail!("invalid floating type or noncanonical bits"),
    }
}

pub fn verify(oracle: &Value, results: &[Val]) -> Result<()> {
    if oracle["kind"] != "float_bits_v1"
        || oracle["expected_trap"]
            .as_str()
            .is_some_and(|s| !s.is_empty())
    {
        bail!("invalid float oracle");
    }
    let p = &oracle["float"];
    let types = p["types"]
        .as_array()
        .ok_or_else(|| anyhow!("missing float types"))?;
    let expected = oracle["expected"]
        .as_array()
        .ok_or_else(|| anyhow!("missing float expectations"))?;
    if types.is_empty() || types.len() != expected.len() || results.len() != expected.len() {
        bail!("float result count mismatch");
    }
    let abs = if p["absolute_tolerance"].is_null() {
        0.0
    } else {
        p["absolute_tolerance"]
            .as_f64()
            .ok_or_else(|| anyhow!("invalid absolute tolerance"))?
    };
    let rel = if p["relative_tolerance"].is_null() {
        0.0
    } else {
        p["relative_tolerance"]
            .as_f64()
            .ok_or_else(|| anyhow!("invalid relative tolerance"))?
    };
    if !abs.is_finite() || !rel.is_finite() || abs < 0.0 || rel < 0.0 {
        bail!("invalid float tolerances");
    }
    let nan = p["nan"].as_str().unwrap_or("");
    let zero = p["signed_zero"].as_str().unwrap_or("");
    if !["reject", "any_nan"].contains(&nan) || !["match", "ignore"].contains(&zero) {
        bail!("explicit float policies required");
    }
    for (i, result) in results.iter().enumerate() {
        let typ = types[i]
            .as_str()
            .ok_or_else(|| anyhow!("invalid float type"))?;
        let want = decode(typ, bits(&expected[i])?)?;
        let got = match (typ, result) {
            ("f32", Val::F32(n)) => f32::from_bits(*n) as f64,
            ("f64", Val::F64(n)) => f64::from_bits(*n),
            _ => bail!("float result type mismatch"),
        };
        let matched = if want.is_nan() {
            nan == "any_nan" && got.is_nan()
        } else if got.is_nan() {
            false
        } else if want.is_infinite() || got.is_infinite() {
            got == want
        } else if got == 0.0 && want == 0.0 {
            zero == "ignore" || got.is_sign_negative() == want.is_sign_negative()
        } else if got == want {
            true
        } else {
            let delta = (got - want).abs();
            let scale = got.abs().max(want.abs());
            let relative = if delta.is_infinite() {
                (got / scale - want / scale).abs()
            } else {
                delta / scale
            };
            delta <= abs || relative <= rel
        };
        if !matched {
            bail!("incorrect result: floating result {i}");
        }
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;
    fn oracle(want: f64) -> Value {
        json!({"kind":"float_bits_v1","expected":[want.to_bits().to_string()],"float":{"types":["f64"],"absolute_tolerance":1e-12,"relative_tolerance":1e-9,"nan":"reject","signed_zero":"match"}})
    }
    #[test]
    fn numeric_edges() {
        for (got, want, ok) in [
            (1.0 + 1e-10, 1.0, true),
            (1.01, 1.0, false),
            (1e-13, 0.0, true),
            (f64::MAX, -f64::MAX, false),
            (f64::INFINITY, f64::INFINITY, true),
            (f64::NEG_INFINITY, f64::INFINITY, false),
            (-0.0, 0.0, false),
            (f64::NAN, 1.0, false),
        ] {
            assert_eq!(
                verify(&oracle(want), &[Val::F64(got.to_bits())]).is_ok(),
                ok
            );
        }
        let mut o = oracle(f64::NAN);
        o["float"]["nan"] = json!("any_nan");
        assert!(verify(&o, &[Val::F64(0x7ff8000000000042)]).is_ok());
        assert!(verify(&oracle(0.0), &[Val::I64(0)]).is_err());
    }
}
