-- Run in a report directory generated with timed-work-launch-rates-v2.
-- One eligible row is one independent process, not one timed batch.
-- Exact integer totals are decimal strings: do not cast them to DOUBLE when
-- checking exact work counts, or assume they fit in a fixed-width integer.
CREATE OR REPLACE TEMP VIEW throughput_evidence AS
SELECT * FROM read_parquet('throughput.parquet');

SELECT run, runtime, workload, scenario, profile, work_unit,
       units_per_invocation_decimal,
       count(*) AS independent_launches,
       median(work_units_per_second) AS median_work_units_per_second
FROM throughput_evidence
WHERE throughput_status = 'available' AND trial_status = 'ok'
GROUP BY ALL ORDER BY run, workload, scenario, runtime;

-- Separate coverage result: never silently discard failures/unsupported cells.
SELECT run, runtime, workload, scenario, profile, work_unit,
       units_per_invocation_decimal, throughput_status, trial_status,
       count(*) AS trial_records
FROM throughput_evidence
GROUP BY ALL ORDER BY run, workload, scenario, runtime;

-- These are timed-region rates, not sustained service capacity. This query
-- supplies no confidence intervals; use the versioned JSON bootstrap results.
