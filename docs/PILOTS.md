# Fixed-budget pilot workflow

`pilot-plan` uses a sealed timing run to choose a fixed repetition budget for a
separate confirmation run. It does not qualify the machine, make official
publication available, prove convergence, or guarantee statistical precision.

```sh
./bin/wasmbench run --suite core --runtimes wazero,v8 \
  --scenarios compile --launches 6 --samples 10 --operations 1 \
  --warmup 0 --out runs/pilot
./bin/wasmbench pilot-plan --run runs/pilot \
  --relative-half-width 0.05 --min-launches 6 --max-launches 100 \
  --out budget.json
# Inspect status and every cell before starting the fixed confirmation budget.
./bin/wasmbench pilot-run --plan budget.json --out runs/confirmation
./bin/wasmbench report --run runs/confirmation --out reports/confirmation
```

Build the runner/adapters before the pilot and keep those exact bytes for
confirmation. Existing runner, adapter and artifact hashes remain enforced.
The pilot source path is absolute and must remain available when executing the
plan. The confirmation bundle embeds the full plan, source lock/checksum digests
and summaries; subsequent replay uses the normal locked-bundle workflow.
Plan files and output directories are new-only. Confirmation is added to the
local run index, uses fresh adapter processes and never pools pilot samples.

## Selection rule and limits

`launch-median-ci-budget-v1` uses the existing versioned percentile bootstrap
over independent process medians. For each declared runtime/workload/scenario
cell, relative half-width is the larger distance from the pilot median to either
95% interval endpoint, divided by the median. The candidate budget is:

`ceil(pilot_launches * (observed_relative_half_width / target)^2)`

This square-root scaling is a planning heuristic, not a median-specific coverage
guarantee, effect-size power analysis, or multiple-comparison correction. Small
pilots can miss noise and rare modes; a zero observed width only selects the
minimum budget, not certainty. The target concerns each cell's launch-median
interval, not a paired runtime-ratio interval. Confirmation must still report its
actual uncertainty even when wider than the planning target.

The common budget is the maximum candidate across **all** declared cells, bounded
below by the explicit minimum (at least six). If any estimate exceeds the cap,
the plan is unresolved: it never clips the estimate and implies precision was
met. Failed/unsupported/missing cells, failed admission, duplicate/out-of-range
blocks, unverified samples, nonpositive medians, fewer than six successful pilot
launches and detected within-launch drift also prevent a runnable plan. Every
cell and its outcome remain in the output. No fastest-looking subset is chosen.

`pilot-run` rereads checksum-verified pilot evidence and recomputes the entire
decision before creating output. Edited or stale decisions and unresolved plans
are rejected. It accepts no launch-count override. The embedded decision and
launch count must agree when validating/loading the confirmation lock. A
confirmation bundle cannot be recycled through `pilot-plan`; run a separately
declared pilot if the experiment changes. These checks prevent accidental
adaptive reuse, not a malicious operator forging an unsigned run.

The runner checks the same host name, OS, architecture, CPU description, kernel,
page size and recorded runtime environment. This is **not** a dedicated-machine
certificate: frequency, affinity, competing activity and the rest of the host
policy still need qualification. Official publication remains unavailable until
those independent controls and gates are implemented and verified.
