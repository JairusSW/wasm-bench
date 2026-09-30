-- Run in a report directory. Do not combine collectors, phases, scopes,
-- instrumentation profiles or normalization denominators. Values are not
-- divided by operations: footprint and allocation activity differ.
CREATE OR REPLACE TEMP VIEW observations AS
SELECT * FROM read_parquet('observations.parquet');
CREATE OR REPLACE TEMP VIEW observation_launches AS
SELECT run, trial, runtime, workload, scenario, trial_profile, metric,
       definition_version, unit, scope, phase, collector, collector_version,
       quality, instrumentation_profile, normalization_denominator, operations,
       sample_index IS NULL AS trial_level,
       median(value) AS launch_value
FROM observations
WHERE block >= 0 AND trial_status = 'ok' AND status = 'available'
  AND value IS NOT NULL
  AND (sample_index IS NULL OR (verified AND NOT warmup))
GROUP BY ALL;
SELECT runtime, workload, scenario, trial_profile, metric, definition_version,
       unit, scope, phase, collector, collector_version, quality,
       instrumentation_profile, normalization_denominator, operations,
       trial_level, count(*) AS independent_launches,
       median(launch_value) AS median_value
FROM observation_launches GROUP BY ALL
ORDER BY workload, scenario, metric, runtime;
-- Keep unavailable metrics visible separately from successful measurements.
SELECT runtime, workload, scenario, metric, scope, phase, collector, quality,
       status, reason, trial_status, count(DISTINCT trial) AS launches
FROM observations WHERE block >= 0 GROUP BY ALL
ORDER BY workload, metric, runtime;
