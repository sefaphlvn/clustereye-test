-- Users tablosu
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    username VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(60) NOT NULL,
    email VARCHAR(255) UNIQUE,
    status VARCHAR(20) DEFAULT 'active',
    admin VARCHAR(5) DEFAULT 'false',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Örnek kullanıcı (admin/admin123)
-- Şifre: admin123 (bcrypt hash'i)
INSERT INTO users (username, password_hash, email, status, admin) 
VALUES ('admin', '$2a$10$3Qxl2hvTB3Xb.UUJJkfYAO8SdXFQzXS5G.zeeZ5UsAOj9QzKwwEmW', 'admin@example.com', 'active', 'true')
ON CONFLICT (username) DO NOTHING; 