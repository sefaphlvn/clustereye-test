-- Notification Settings tablosu
CREATE TABLE IF NOT EXISTS notification_settings (
    id SERIAL PRIMARY KEY,
    slack_webhook_url TEXT,
    slack_enabled BOOLEAN DEFAULT false,
    email_enabled BOOLEAN DEFAULT false,
    email_server TEXT,
    email_port TEXT,
    email_user TEXT,
    email_password TEXT,
    email_from TEXT,
    email_recipients TEXT[],
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- İlk notification settings kaydını oluştur
INSERT INTO notification_settings (
    slack_webhook_url,
    slack_enabled,
    email_enabled,
    email_server,
    email_port,
    email_user,
    email_password,
    email_from,
    email_recipients
) VALUES (
    '',
    false,
    false,
    '',
    '',
    '',
    '',
    '',
    ARRAY[]::TEXT[]
) ON CONFLICT DO NOTHING; 