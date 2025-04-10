-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS mongo_data (
    id SERIAL PRIMARY KEY,
    jsondata JSONB NOT NULL, -- MongoDB cluster ve node bilgilerini JSON formatında saklar
    clustername VARCHAR(255) NOT NULL, -- Cluster adı
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_mongo_data_clustername ON mongo_data(clustername);
CREATE INDEX idx_mongo_data_created_at ON mongo_data(created_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS mongo_data;
-- +goose StatementEnd 