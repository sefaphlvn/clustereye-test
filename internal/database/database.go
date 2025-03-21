package database

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

// Config, veritabanı bağlantı ayarlarını içerir
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// ConnectDatabase, verilen konfigürasyonla PostgreSQL veritabanına bağlanır
func ConnectDatabase(cfg Config) (*sql.DB, error) {
	// Bağlantı dizesi oluştur
	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode,
	)

	// Veritabanına bağlan
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("veritabanı açılamadı: %w", err)
	}

	// Bağlantıyı test et
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("veritabanı bağlantı testi başarısız: %w", err)
	}

	return db, nil
}

// InitSchema, veritabanı şemasını başlatır
func InitSchema(db *sql.DB) error {
	// Agent tablosunu oluştur
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS agents (
			id SERIAL PRIMARY KEY,
			key TEXT NOT NULL,
			agent_id TEXT NOT NULL UNIQUE,
			hostname TEXT NOT NULL,
			ip TEXT NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("agents tablosu oluşturulamadı: %w", err)
	}

	// PostgreSQL veri tablosunu oluştur
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS postgres_data (
			id SERIAL PRIMARY KEY,
			jsondata JSONB NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("postgres_data tablosu oluşturulamadı: %w", err)
	}

	return nil
} 