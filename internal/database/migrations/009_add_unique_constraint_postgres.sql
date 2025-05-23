-- +goose Up
-- +goose StatementBegin

-- Önce duplicate kayıtları temizle, en son güncellenen kaydı sakla
DELETE FROM postgres_data 
WHERE id NOT IN (
    SELECT DISTINCT ON (clustername) id 
    FROM postgres_data 
    ORDER BY clustername, updated_at DESC
);

-- Clustername için unique constraint ekle
ALTER TABLE postgres_data ADD CONSTRAINT unique_postgres_clustername UNIQUE (clustername);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Unique constraint'i kaldır
ALTER TABLE postgres_data DROP CONSTRAINT IF EXISTS unique_postgres_clustername;

-- +goose StatementEnd 