-- +goose Up
-- +goose StatementBegin

-- Önce duplicate kayıtları temizle, en son güncellenen kaydı sakla
DELETE FROM mongo_data 
WHERE id NOT IN (
    SELECT DISTINCT ON (clustername) id 
    FROM mongo_data 
    ORDER BY clustername, updated_at DESC
);

-- Clustername için unique constraint ekle
ALTER TABLE mongo_data ADD CONSTRAINT unique_mongo_clustername UNIQUE (clustername);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Unique constraint'i kaldır
ALTER TABLE mongo_data DROP CONSTRAINT IF EXISTS unique_mongo_clustername;

-- +goose StatementEnd 