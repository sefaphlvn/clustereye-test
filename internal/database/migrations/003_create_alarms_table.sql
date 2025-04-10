-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS alarms (
    id SERIAL PRIMARY KEY,
    alarm_id VARCHAR(255) NOT NULL, -- Alarm'ın tanımlayıcı ID'si
    event_id VARCHAR(255) NOT NULL, -- Alarm olayının benzersiz ID'si
    agent_id VARCHAR(255) NOT NULL, -- Alarmın geldiği agent
    status VARCHAR(50) NOT NULL,    -- "triggered", "resolved"
    metric_name VARCHAR(255) NOT NULL, -- Metrik adı
    metric_value VARCHAR(255) NOT NULL, -- Metrik değeri
    message TEXT NOT NULL,          -- Alarm mesajı
    severity VARCHAR(50) NOT NULL,  -- "critical", "warning", "info"
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_alarms_agent_id ON alarms(agent_id);
CREATE INDEX idx_alarms_status ON alarms(status);
CREATE INDEX idx_alarms_created_at ON alarms(created_at);
CREATE INDEX idx_alarms_severity ON alarms(severity);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS alarms;
-- +goose StatementEnd 