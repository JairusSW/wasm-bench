-- Run from the source HTML report directory using DuckDB.
-- Preserve failure and warmup rows in the file; filter only for this summary.
-- Configuration identity prevents pooling unrelated experiments.
SELECT config_sha256, profile, variant, status, count(*) AS builds,
       median(summed_tool_process_wall_ns) AS diagnostic_wall_ns,
       median(summed_tool_wait_cpu_ns) AS wait_cpu_ns,
       median(max_step_cgroup_peak_bytes) AS max_step_peak_bytes
FROM read_parquet('compiler-builds.parquet')
WHERE stage = 'trial' AND NOT warmup
GROUP BY config_sha256, profile, variant, status
ORDER BY config_sha256, profile, variant, status;
