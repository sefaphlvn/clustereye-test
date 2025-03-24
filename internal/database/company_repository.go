package database

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"
)

var (
	ErrCompanyNotFound = errors.New("firma bulunamadı")
	ErrKeyExpired      = errors.New("agent anahtarı süresi dolmuş")
)

// Company, firma bilgilerini temsil eder
type Company struct {
	ID             int       `json:"id"`
	CompanyName    string    `json:"company_name"`
	AgentKey       string    `json:"agent_key"`
	ExpirationDate time.Time `json:"expiration_date"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Agent, agent bilgilerini temsil eder
type Agent struct {
	ID        int       `json:"id"`
	CompanyID int       `json:"company_id"`
	AgentID   string    `json:"agent_id"`
	Hostname  string    `json:"hostname"`
	IP        string    `json:"ip"`
	LastSeen  time.Time `json:"last_seen"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CompanyRepository, firma ve agent verilerini yönetir
type CompanyRepository struct {
	db *sql.DB
}

// NewCompanyRepository, yeni bir CompanyRepository oluşturur
func NewCompanyRepository(db *sql.DB) *CompanyRepository {
	return &CompanyRepository{db: db}
}

// ValidateAgentKey, agent anahtarının geçerli olup olmadığını kontrol eder
func (r *CompanyRepository) ValidateAgentKey(ctx context.Context, agentKey string) (*Company, error) {
	query := `
		SELECT id, company_name, agent_key, expiration_date, created_at, updated_at
		FROM companies
		WHERE agent_key = $1
	`

	var company Company
	err := r.db.QueryRowContext(ctx, query, agentKey).Scan(
		&company.ID,
		&company.CompanyName,
		&company.AgentKey,
		&company.ExpirationDate,
		&company.CreatedAt,
		&company.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrCompanyNotFound
		}
		return nil, err
	}

	// Süre kontrolü
	if time.Now().After(company.ExpirationDate) {
		return nil, ErrKeyExpired
	}

	return &company, nil
}

// RegisterAgent, yeni bir agent kaydeder veya mevcut olanı günceller
func (r *CompanyRepository) RegisterAgent(ctx context.Context, companyID int, agentID, hostname, ip string) error {
	query := `
		INSERT INTO agents (company_id, agent_id, hostname, ip, last_seen, status)
		VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP, 'active')
		ON CONFLICT (agent_id) 
		DO UPDATE SET 
			hostname = $3,
			ip = $4,
			last_seen = CURRENT_TIMESTAMP,
			status = 'active',
			updated_at = CURRENT_TIMESTAMP
		RETURNING id
	`

	_, err := r.db.ExecContext(ctx, query, companyID, agentID, hostname, ip)
	return err
}

// SavePostgresConnInfo, PostgreSQL bağlantı bilgilerini kaydeder
func (r *CompanyRepository) SavePostgresConnInfo(ctx context.Context, hostname, cluster, username, password string) error {
	log.Printf("SavePostgresConnInfo çağrıldı: hostname=%s, cluster=%s, user=%s", hostname, cluster, username)

	// Önce kaydın var olup olmadığını kontrol et
	var count int
	countQuery := "SELECT COUNT(*) FROM postgres_conninfo WHERE nodename = $1"
	err := r.db.QueryRowContext(ctx, countQuery, hostname).Scan(&count)
	if err != nil {
		log.Printf("Kayıt sayısı sorgusu hatası: %v", err)
		return err
	}

	log.Printf("Mevcut kayıt sayısı: %d", count)

	var query string
	var args []interface{}

	if count > 0 {
		// Kayıt varsa güncelle
		query = `
			UPDATE postgres_conninfo 
			SET clustername = $2, username = $3, password = $4, send_diskalert = true, silence_until = NULL
			WHERE nodename = $1
		`
		args = []interface{}{hostname, cluster, username, password}
		log.Printf("Güncelleme sorgusu hazırlandı")
	} else {
		// Kayıt yoksa ekle
		query = `
			INSERT INTO postgres_conninfo (nodename, clustername, username, password, send_diskalert)
			VALUES ($1, $2, $3, $4, true)
		`
		args = []interface{}{hostname, cluster, username, password}
		log.Printf("Ekleme sorgusu hazırlandı")
	}

	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		log.Printf("Sorgu yürütme hatası: %v", err)
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	log.Printf("Etkilenen satır sayısı: %d", rowsAffected)

	return nil
}
