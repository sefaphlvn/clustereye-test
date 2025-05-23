-- +goose Up
-- +goose StatementBegin

-- Acknowledged alanı için index (en çok kullanılan filtre)
CREATE INDEX IF NOT EXISTS idx_alarms_acknowledged ON alarms (acknowledged);

-- Composite index: acknowledged + created_at (sık kullanılan kombinasyon)
CREATE INDEX IF NOT EXISTS idx_alarms_acknowledged_created_at ON alarms (acknowledged, created_at DESC);

-- Metric name için index (filtering için)
CREATE INDEX IF NOT EXISTS idx_alarms_metric_name ON alarms (metric_name);

-- Composite index: severity + created_at (severity filtreleme için)
CREATE INDEX IF NOT EXISTS idx_alarms_severity_created_at ON alarms (severity, created_at DESC);

-- Agent ID ve created_at composite index (agent bazlı filtreleme için)
CREATE INDEX IF NOT EXISTS idx_alarms_agent_id_created_at ON alarms (agent_id, created_at DESC);

-- Performans için partial index: sadece unacknowledged alarmlar
CREATE INDEX IF NOT EXISTS idx_alarms_unacknowledged_recent ON alarms (created_at DESC) 
WHERE acknowledged = false;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Index'leri kaldır
DROP INDEX IF EXISTS idx_alarms_acknowledged;
DROP INDEX IF EXISTS idx_alarms_acknowledged_created_at;
DROP INDEX IF EXISTS idx_alarms_metric_name;
DROP INDEX IF EXISTS idx_alarms_severity_created_at;
DROP INDEX IF EXISTS idx_alarms_agent_id_created_at;
DROP INDEX IF EXISTS idx_alarms_unacknowledged_recent;

-- +goose StatementEnd 