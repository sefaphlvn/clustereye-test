-- Add database column to alarms table
ALTER TABLE alarms 
ADD COLUMN IF NOT EXISTS database VARCHAR(255);

-- Update existing alarms for postgresql_slow_queries to have a default database
UPDATE alarms
SET database = 'postgres'
WHERE metric_name = 'postgresql_slow_queries' AND database IS NULL;

-- Make sure alarms table exists
CREATE TABLE IF NOT EXISTS alarms (
    id SERIAL PRIMARY KEY,
    alarm_id VARCHAR(255) NOT NULL,
    event_id VARCHAR(255) NOT NULL,
    agent_id VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL,
    metric_name VARCHAR(255) NOT NULL,
    metric_value TEXT,
    message TEXT,
    severity VARCHAR(50),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    acknowledged BOOLEAN DEFAULT FALSE,
    database VARCHAR(255)
);

-- Add index for faster queries
CREATE INDEX IF NOT EXISTS idx_alarms_metric_name ON alarms (metric_name);
CREATE INDEX IF NOT EXISTS idx_alarms_database ON alarms (database); 