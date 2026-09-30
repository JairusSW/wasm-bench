# Observed host baselines

Capture the intended host and recorded environment before planning:

```sh
wasmbench host-policy --out host.json
wasmbench run --suite core --runtimes v8 --host-policy host.json --out runs/baseline
```

The new-only JSON file is embedded in the experiment lock. `run`, `check`, and
`plan` accept it; existing locks cannot override it. Reproduction preserves it.
Before run preparation, a mismatch refuses execution before creating the output
directory. After trials, the runner captures a second observation. A mismatch
seals a diagnostic bundle, prohibits performance publication, and returns an
error. Raw correctness outcomes and samples remain intact; derived latency,
memory, scaling, and aggregate analyses exclude affected trials. Offline loading
recomputes both checks rather than trusting their status strings.

This is boundary-only comparison, not continuous monitoring, dedicated-machine
certification, or automatic configuration. Matching unavailable facts and
uncontrolled settings does not establish control. Changes between observations
can go undetected. Only fields recorded by `IdentifyHost` are compared, including
the four recorded environment variables and available Linux fingerprint facts.
Runtime end-mismatch bundles can be indexed explicitly with
`wasmbench index --run PATH`.

Source-build benchmarks accept the same `--host-policy host.json` option on
`source-bench`. Their configuration hash includes the baseline, and
`source-bench-replay` preserves it without overrides. Checks bracket the complete
benchmark (including admission and warmup), not each compiler process. Existing
per-build host checks remain in place. An end mismatch seals a diagnostic bundle
and returns an error; timing, CPU and memory summaries exclude all its trials.
Offline verification recomputes both checks even for admission-failure bundles.
Standalone `source-build` receipts do not provide this two-boundary contract.
