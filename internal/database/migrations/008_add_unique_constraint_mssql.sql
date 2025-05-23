-- +goose Up
-- +goose StatementBegin

-- Önce duplicate kayıtları temizle, en son güncellenen kaydı sakla
DELETE FROM mssql_data 
WHERE id NOT IN (
    SELECT DISTINCT ON (clustername) id 
    FROM mssql_data 
    ORDER BY clustername, updated_at DESC
);

-- Clustername için unique constraint ekle
ALTER TABLE mssql_data ADD CONSTRAINT unique_mssql_clustername UNIQUE (clustername);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Unique constraint'i kaldır
ALTER TABLE mssql_data DROP CONSTRAINT IF EXISTS unique_mssql_clustername;

-- +goose StatementEnd 