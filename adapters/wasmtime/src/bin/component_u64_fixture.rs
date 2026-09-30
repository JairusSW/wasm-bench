use std::io::Write;
fn main() -> anyhow::Result<()> {
    let args: Vec<_> = std::env::args_os().collect();
    anyhow::ensure!(args.len() == 2, "provide one new output artifact path");
    let bytes = wat::parse_str(include_str!(
        "../../../../recipes/fixtures/component-u64.wat"
    ))?;
    let mut output = std::fs::OpenOptions::new()
        .write(true)
        .create_new(true)
        .open(&args[1])?;
    output.write_all(&bytes)?;
    output.sync_all()?;
    Ok(())
}
