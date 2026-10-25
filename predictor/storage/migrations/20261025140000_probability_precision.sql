ALTER TABLE metric_observations
    MODIFY COLUMN probability Float64;

ALTER TABLE company_analyses
    MODIFY COLUMN probability Float64;
