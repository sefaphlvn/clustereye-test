-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS mssql_data (
    id SERIAL PRIMARY KEY,
    jsondata JSONB NOT NULL, -- MSSQL cluster and node information stored in JSON format
    clustername VARCHAR(255) NOT NULL, -- Cluster name
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_mssql_data_clustername ON mssql_data(clustername);
CREATE INDEX idx_mssql_data_created_at ON mssql_data(created_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS mssql_data;
-- +goose StatementEnd 