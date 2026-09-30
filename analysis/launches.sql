-- DuckDB analysis preserves process replication before aggregating.
-- Run from a generated report directory:
--   duckdb -c ".read /path/to/wasm-bench/analysis/launches.sql"
-- Requires sample-evidence-parquet-v2 (regenerate old reports without altering
-- their source bundles). Diagnostic/raw timers remain in evidence, not medians.
CREATE OR REPLACE TEMP VIEW evidence AS SELECT * FROM read_parquet('samples.parquet');
CREATE OR REPLACE TEMP VIEW launch_medians AS
SELECT runtime, workload, scenario, profile, block,
       median(elapsed_ns::DOUBLE / operations) AS ns_per_operation
FROM evidence
WHERE latency_eligible AND profile = 'timing'
  AND status = 'ok' AND verified AND NOT warmup AND block >= 0
  AND operations > 0 AND elapsed_ns >= 0
GROUP BY runtime, workload, scenario, profile, block;
SELECT runtime, workload, scenario, profile, count(*) AS independent_launches,
       median(ns_per_operation) AS median_ns, avg(ns_per_operation) AS mean_ns,
       stddev_samp(ns_per_operation) AS launch_dispersion_ns
FROM launch_medians GROUP BY ALL ORDER BY workload, scenario, runtime;
-- Coverage remains a separate result, including unsupported and failed trials.
SELECT runtime, workload, scenario, status, count(DISTINCT trial) AS launches
FROM evidence WHERE block >= 0 GROUP BY ALL ORDER BY workload, scenario, runtime;
