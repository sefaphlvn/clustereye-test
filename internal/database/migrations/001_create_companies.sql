-- companies tablosunu oluştur
CREATE TABLE IF NOT EXISTS companies (
    id SERIAL PRIMARY KEY,
    company_name VARCHAR(255) NOT NULL,
    agent_key VARCHAR(64) NOT NULL UNIQUE,
    expiration_date TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- agents tablosunu oluştur (agent kayıtları için)
CREATE TABLE IF NOT EXISTS agents (
    id SERIAL PRIMARY KEY,
    company_id INTEGER REFERENCES companies(id),
    agent_id VARCHAR(64) NOT NULL UNIQUE,
    hostname VARCHAR(255) NOT NULL,
    ip VARCHAR(45) NOT NULL,
    last_seen TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR(20) DEFAULT 'active',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Örnek bir firma kaydı ekle
INSERT INTO companies (company_name, agent_key, expiration_date)
VALUES ('Test Firması', 'xxx-xxx-xxx-xxx', '2025-12-31 23:59:59'); 