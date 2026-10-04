use anyhow::{Result, ensure};
use object::{Object, ObjectSection};
use serde_json::{Value, json};

pub fn inspect(bytes: &[u8], version: &str) -> Result<Value> {
    let file = object::File::parse(bytes)?;
    ensure!(
        file.kind() == object::ObjectKind::Relocatable,
        "expected relocatable compiler object"
    );
    let mut size = 0u64;
    for section in file.sections() {
        if section.kind() != object::SectionKind::Text {
            continue;
        }
        ensure!(
            section.compressed_file_range()?.format == object::CompressionFormat::None,
            "compressed executable sections unsupported"
        );
        let data = section.data()?;
        ensure!(
            data.len() as u64 == section.size(),
            "truncated executable section"
        );
        size = size
            .checked_add(section.size())
            .ok_or_else(|| anyhow::anyhow!("code size overflow"))?;
    }
    ensure!(
        size <= (1u64 << 53),
        "code size cannot be represented exactly"
    );
    let common = json!({"definition_version":1,"unit":"bytes","scope":"compiled_module","phase":"compile","collector":"WAVM/Runtime/getObjectCode/executable-sections","collector_version":version,"quality":"engine_reported","profile":"code","normalization_denominator":"module"});
    let mut available = common.clone();
    available["metric"] = json!("native.code_size");
    available["status"] = json!("available");
    available["value"] = json!(size);
    available["reason"] = json!(
        "Executable sections of the relocatable compiler object, including section padding; excludes object headers, debug data, symbols, relocation tables and unwind metadata. Not the linked live executable image."
    );
    let mut export = common;
    export["metric"] = json!("native.code_export");
    export["status"] = json!("unavailable");
    export["reason"] =
        json!("Relocatable object captured for size; linked native image export is not available");
    Ok(json!({"diagnostics":[available,export]}))
}

#[cfg(test)]
mod tests {
    use super::*;
    const OBJECT: &[u8] = include_bytes!("../tests/fixtures/wavm-call-arm64.o");
    #[test]
    fn excludes_object_metadata() {
        let result = inspect(OBJECT, "nightly-2026-04-05").unwrap();
        assert_eq!(result["diagnostics"][0]["value"], 48);
        assert_eq!(result["diagnostics"][1]["status"], "unavailable");
    }
    #[test]
    fn captures_elf_large_text_sections() {
        let object = include_bytes!("../tests/fixtures/wavm-return42-amd64.o");
        let result = inspect(object, "nightly-2026-04-05").unwrap();
        assert_eq!(result["diagnostics"][0]["value"], 46);
    }
    #[test]
    fn rejects_linked_and_truncated_objects() {
        let mut linked = OBJECT.to_vec();
        linked[12..16].copy_from_slice(&2u32.to_le_bytes());
        assert!(inspect(&linked, "test").is_err());
        assert!(inspect(&OBJECT[..1020], "test").is_err());
        assert!(inspect(b"not an object", "test").is_err());
    }
}
