// Bu dosya artık kullanılmıyor, tüm notification işlemleri handlers.go dosyasına taşındı.

package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// NotificationSettings yapısı
type NotificationSettings struct {
	SlackWebhookURL  string   `json:"slackWebhookUrl"`
	SlackEnabled     bool     `json:"slackEnabled"`
	EmailEnabled     bool     `json:"emailEnabled"`
	EmailServer      string   `json:"emailServer"`
	EmailPort        string   `json:"emailPort"`
	EmailUser        string   `json:"emailUser"`
	EmailPassword    string   `json:"emailPassword"`
	EmailFrom        string   `json:"emailFrom"`
	EmailRecipients  []string `json:"emailRecipients"`
}

// GetNotificationSettings, notification ayarlarını getirir
func GetNotificationSettings(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Sadece admin kullanıcıların erişimine izin ver
		adminValue, exists := c.Get("admin")
		if !exists || adminValue != "true" {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   "Admin yetkisi gerekiyor",
			})
			return
		}

		var settings NotificationSettings
		var recipientsBytes []byte
		err := db.QueryRow(`
			SELECT 
				slack_webhook_url,
				slack_enabled,
				email_enabled,
				email_server,
				email_port,
				email_user,
				email_password,
				email_from,
				email_recipients
			FROM notification_settings
			ORDER BY id DESC
			LIMIT 1
		`).Scan(
			&settings.SlackWebhookURL,
			&settings.SlackEnabled,
			&settings.EmailEnabled,
			&settings.EmailServer,
			&settings.EmailPort,
			&settings.EmailUser,
			&settings.EmailPassword,
			&settings.EmailFrom,
			&recipientsBytes,
		)

		if err != nil {
			if err == sql.ErrNoRows {
				// Eğer kayıt yoksa varsayılan değerleri döndür
				c.JSON(http.StatusOK, gin.H{
					"success": true,
					"settings": NotificationSettings{
						SlackWebhookURL:  "",
						SlackEnabled:     false,
						EmailEnabled:     false,
						EmailServer:      "",
						EmailPort:        "",
						EmailUser:        "",
						EmailPassword:    "",
						EmailFrom:        "",
						EmailRecipients:  []string{},
					},
				})
				return
			}
			log.Printf("Database error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Notification ayarları alınırken bir hata oluştu",
			})
			return
		}

		// PostgreSQL array formatını string array'e dönüştür
		if len(recipientsBytes) > 0 {
			// PostgreSQL array formatı: {value1,value2,...}
			// İlk ve son karakterleri kaldır ({ ve })
			recipientsStr := string(recipientsBytes)
			if len(recipientsStr) >= 2 {
				recipientsStr = recipientsStr[1 : len(recipientsStr)-1]
				if recipientsStr != "" {
					settings.EmailRecipients = strings.Split(recipientsStr, ",")
				} else {
					settings.EmailRecipients = []string{}
				}
			} else {
				settings.EmailRecipients = []string{}
			}
		} else {
			settings.EmailRecipients = []string{}
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"settings": settings,
		})
	}
}

// UpdateNotificationSettings, notification ayarlarını günceller
func UpdateNotificationSettings(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Sadece admin kullanıcıların erişimine izin ver
		adminValue, exists := c.Get("admin")
		if !exists || adminValue != "true" {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   "Admin yetkisi gerekiyor",
			})
			return
		}

		// Önce mevcut ayarları al
		var currentSettings NotificationSettings
		var recipientsBytes []byte
		err := db.QueryRow(`
			SELECT 
				slack_webhook_url,
				slack_enabled,
				email_enabled,
				email_server,
				email_port,
				email_user,
				email_password,
				email_from,
				email_recipients
			FROM notification_settings
			ORDER BY id DESC
			LIMIT 1
		`).Scan(
			&currentSettings.SlackWebhookURL,
			&currentSettings.SlackEnabled,
			&currentSettings.EmailEnabled,
			&currentSettings.EmailServer,
			&currentSettings.EmailPort,
			&currentSettings.EmailUser,
			&currentSettings.EmailPassword,
			&currentSettings.EmailFrom,
			&recipientsBytes,
		)

		if err != nil && err != sql.ErrNoRows {
			log.Printf("Database error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Mevcut ayarlar alınırken bir hata oluştu",
			})
			return
		}

		// PostgreSQL array formatını string array'e dönüştür
		if len(recipientsBytes) > 0 {
			// PostgreSQL array formatı: {value1,value2,...}
			// İlk ve son karakterleri kaldır ({ ve })
			recipientsStr := string(recipientsBytes)
			if len(recipientsStr) >= 2 {
				recipientsStr = recipientsStr[1 : len(recipientsStr)-1]
				if recipientsStr != "" {
					currentSettings.EmailRecipients = strings.Split(recipientsStr, ",")
				} else {
					currentSettings.EmailRecipients = []string{}
				}
			} else {
				currentSettings.EmailRecipients = []string{}
			}
		} else {
			currentSettings.EmailRecipients = []string{}
		}

		// Gelen veriyi parse et
		var updateData map[string]interface{}
		if err := c.ShouldBindJSON(&updateData); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Geçersiz veri formatı",
			})
			return
		}

		// Gelen verileri mevcut ayarlarla birleştir
		if slackWebhookURL, ok := updateData["slack_webhook_url"].(string); ok {
			currentSettings.SlackWebhookURL = slackWebhookURL
		}
		if slackEnabled, ok := updateData["slack_enabled"].(bool); ok {
			currentSettings.SlackEnabled = slackEnabled
		}
		if emailEnabled, ok := updateData["email_enabled"].(bool); ok {
			currentSettings.EmailEnabled = emailEnabled
		}
		if emailServer, ok := updateData["email_server"].(string); ok {
			currentSettings.EmailServer = emailServer
		}
		if emailPort, ok := updateData["email_port"].(string); ok {
			currentSettings.EmailPort = emailPort
		}
		if emailUser, ok := updateData["email_user"].(string); ok {
			currentSettings.EmailUser = emailUser
		}
		if emailPassword, ok := updateData["email_password"].(string); ok {
			currentSettings.EmailPassword = emailPassword
		}
		if emailFrom, ok := updateData["email_from"].(string); ok {
			currentSettings.EmailFrom = emailFrom
		}
		if emailRecipients, ok := updateData["email_recipients"].([]interface{}); ok {
			recipients := make([]string, len(emailRecipients))
			for i, recipient := range emailRecipients {
				if str, ok := recipient.(string); ok {
					recipients[i] = str
				}
			}
			currentSettings.EmailRecipients = recipients
		}

		// PostgreSQL array formatına dönüştür
		recipientsArray := "{}"
		if len(currentSettings.EmailRecipients) > 0 {
			recipientsArray = "{" + strings.Join(currentSettings.EmailRecipients, ",") + "}"
		}

		// Güncelleme sorgusu
		_, err = db.Exec(`
			UPDATE notification_settings
			SET 
				slack_webhook_url = $1,
				slack_enabled = $2,
				email_enabled = $3,
				email_server = $4,
				email_port = $5,
				email_user = $6,
				email_password = $7,
				email_from = $8,
				email_recipients = $9,
				updated_at = CURRENT_TIMESTAMP
			WHERE id = (
				SELECT id FROM notification_settings ORDER BY id DESC LIMIT 1
			)
		`,
			currentSettings.SlackWebhookURL,
			currentSettings.SlackEnabled,
			currentSettings.EmailEnabled,
			currentSettings.EmailServer,
			currentSettings.EmailPort,
			currentSettings.EmailUser,
			currentSettings.EmailPassword,
			currentSettings.EmailFrom,
			recipientsArray,
		)

		if err != nil {
			log.Printf("Database error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Notification ayarları güncellenirken bir hata oluştu",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Notification ayarları başarıyla güncellendi",
		})
	}
}

// TestSlackNotification, Slack webhook'unu test eder
func TestSlackNotification(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Sadece admin kullanıcıların erişimine izin ver
		adminValue, exists := c.Get("admin")
		if !exists || adminValue != "true" {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   "Admin yetkisi gerekiyor",
			})
			return
		}

		// Mevcut Slack ayarlarını al
		var settings NotificationSettings
		err := db.QueryRow(`
			SELECT slack_webhook_url, slack_enabled
			FROM notification_settings
			ORDER BY id DESC
			LIMIT 1
		`).Scan(&settings.SlackWebhookURL, &settings.SlackEnabled)

		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"error":   "Slack ayarları bulunamadı",
				})
				return
			}
			log.Printf("Database error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Slack ayarları alınırken bir hata oluştu",
			})
			return
		}

		if !settings.SlackEnabled {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Slack bildirimleri devre dışı",
			})
			return
		}

		if settings.SlackWebhookURL == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Slack webhook URL'si ayarlanmamış",
			})
			return
		}

		// Test mesajını hazırla
		message := map[string]interface{}{
			"text": "🔔 *Test Bildirimi*\nBu bir test mesajıdır. Slack webhook'unuz başarıyla çalışıyor!",
		}

		// JSON'a dönüştür
		jsonMessage, err := json.Marshal(message)
		if err != nil {
			log.Printf("JSON marshal error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Mesaj hazırlanırken bir hata oluştu",
			})
			return
		}

		// Slack'e gönder
		resp, err := http.Post(settings.SlackWebhookURL, "application/json", bytes.NewBuffer(jsonMessage))
		if err != nil {
			log.Printf("Slack API error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Slack'e mesaj gönderilirken bir hata oluştu",
			})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Slack webhook'u yanıt vermedi",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Test mesajı başarıyla gönderildi",
		})
	}
}
