-- Run from an offline report directory in DuckDB. Raw integer evidence is never
-- converted to DOUBLE, scaled for multiplexing, or summed across partial CPUs.
CREATE OR REPLACE TEMP VIEW counter_evidence AS
SELECT * FROM read_parquet('counters.parquet');

-- Preserve unsupported trials and unavailable windows beside event coverage.
-- Rows are not independent statistical replicates; block identifies a launch.
SELECT runtime, workload, scenario, trial_status, row_kind, sample_warmup, sample_operations,
       window_status, reading_status, count(*) AS evidence_rows,
       count(DISTINCT trial) AS trials
FROM counter_evidence
WHERE block >= 0
GROUP BY ALL
ORDER BY runtime, workload, scenario, trial_status, row_kind,
         window_status, reading_status;

-- Diagnostic per-CPU records only: still not proof of stable host topology or
-- complete event coverage. Retain all statuses rather than hiding failures.
SELECT trial, block, sample_index, sample_verified, sample_warmup, sample_operations, phase, cpu,
       event, event_type, event_config, raw_count,
       time_enabled_ns, time_running_ns,
       trial_status, window_status, reading_status, reading_reason,
       scope, privilege_scope, collector_version
FROM counter_evidence
WHERE row_kind = 'reading'
ORDER BY trial, window_index, reading_index;
