-- Jobs tablosunu oluştur
CREATE TABLE IF NOT EXISTS jobs (
    job_id VARCHAR(36) PRIMARY KEY,
    type VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL,
    agent_id VARCHAR(50) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
    error_message TEXT,
    parameters JSONB,
    result TEXT,
    CONSTRAINT fk_agent
        FOREIGN KEY(agent_id)
        REFERENCES agents(id)
        ON DELETE CASCADE
);

-- İndeksler
CREATE INDEX idx_jobs_agent_id ON jobs(agent_id);
CREATE INDEX idx_jobs_status ON jobs(status);
CREATE INDEX idx_jobs_type ON jobs(type);
CREATE INDEX idx_jobs_created_at ON jobs(created_at);

-- Job durumlarını izlemek için view
CREATE OR REPLACE VIEW job_statistics AS
SELECT 
    type,
    status,
    COUNT(*) as count,
    MAX(created_at) as last_created,
    MAX(updated_at) as last_updated
FROM jobs
GROUP BY type, status; 