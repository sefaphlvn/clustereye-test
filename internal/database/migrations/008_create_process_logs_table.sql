-- +migrate Up
CREATE TABLE IF NOT EXISTS process_logs (
    id SERIAL PRIMARY KEY,
    process_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    process_type TEXT NOT NULL,  -- postgresql_promotion, postgresql_failover, etc.
    status TEXT NOT NULL,        -- running, completed, failed
    log_messages JSONB NOT NULL, -- Array of log messages
    elapsed_time_s REAL,         -- Elapsed time in seconds
    metadata JSONB,              -- Additional process metadata
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Create indexes for efficient querying
CREATE INDEX IF NOT EXISTS idx_process_logs_process_id ON process_logs(process_id);
CREATE INDEX IF NOT EXISTS idx_process_logs_agent_id ON process_logs(agent_id);
CREATE INDEX IF NOT EXISTS idx_process_logs_process_type ON process_logs(process_type);
CREATE INDEX IF NOT EXISTS idx_process_logs_status ON process_logs(status);
CREATE INDEX IF NOT EXISTS idx_process_logs_updated_at ON process_logs(updated_at);

-- +migrate Down
DROP TABLE IF EXISTS process_logs; 