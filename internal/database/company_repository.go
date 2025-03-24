package database

import (
	"context"
	"database/sql"
	"errors"
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
