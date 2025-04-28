-- Create agent_versions table
CREATE TABLE IF NOT EXISTS agent_versions (
    id SERIAL PRIMARY KEY,
    agent_id VARCHAR(255) NOT NULL UNIQUE,
    version VARCHAR(50) NOT NULL,
    platform VARCHAR(50) NOT NULL,
    architecture VARCHAR(50) NOT NULL,
    hostname VARCHAR(255) NOT NULL,
    os_version VARCHAR(255) NOT NULL,
    go_version VARCHAR(50) NOT NULL,
    reported_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Add indexes
CREATE INDEX IF NOT EXISTS idx_agent_versions_agent_id ON agent_versions(agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_versions_reported_at ON agent_versions(reported_at);

-- Add trigger to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_agent_versions_updated_at
    BEFORE UPDATE ON agent_versions
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column(); 