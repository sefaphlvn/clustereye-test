package api

import (
	"crypto/rand"
	"database/sql"
	"encoding/base32"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

// Generate2FASecret, kullanıcı için 2FA secret oluşturur
func Generate2FASecret(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Kullanıcı kimlik doğrulaması kontrolü
		username, exists := c.Get("username")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		// Kullanıcı bilgilerini al
		var userID int
		var email string
		err := db.QueryRow("SELECT id, email FROM users WHERE username = $1", username).Scan(&userID, &email)
		if err != nil {
			log.Printf("Database error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}

		// TOTP secret oluştur
		key, err := totp.Generate(totp.GenerateOpts{
			Issuer:      "ClusterEye",
			AccountName: fmt.Sprintf("%s (%s)", username, email),
			SecretSize:  32,
		})
		if err != nil {
			log.Printf("TOTP generation error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not generate 2FA secret"})
			return
		}

		// Secret'ı veritabanına kaydet (henüz aktif değil)
		_, err = db.Exec("UPDATE users SET totp_secret = $1 WHERE id = $2", key.Secret(), userID)
		if err != nil {
			log.Printf("Database error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not save 2FA secret"})
			return
		}

		// QR kod URL'ini döndür
		c.JSON(http.StatusOK, gin.H{
			"success":    true,
			"secret":     key.Secret(),
			"qr_code":    key.URL(),
			"manual_key": key.Secret(),
		})
	}
}

// Enable2FA, kullanıcının 2FA'sını aktif eder
func Enable2FA(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Kullanıcı kimlik doğrulaması kontrolü
		username, exists := c.Get("username")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		var request struct {
			Code string `json:"code" binding:"required"`
		}

		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}

		// Kullanıcı bilgilerini al
		var userID int
		var totpSecret string
		err := db.QueryRow("SELECT id, totp_secret FROM users WHERE username = $1", username).Scan(&userID, &totpSecret)
		if err != nil {
			log.Printf("Database error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}

		if totpSecret == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "2FA secret not generated"})
			return
		}

		// TOTP kodunu doğrula
		valid := totp.Validate(request.Code, totpSecret)
		if !valid {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid 2FA code"})
			return
		}

		// Backup kodları oluştur
		backupCodes := generateBackupCodes(10)

		// 2FA'yı aktif et ve backup kodları kaydet
		_, err = db.Exec("UPDATE users SET totp_enabled = true, backup_codes = $1 WHERE id = $2",
			pq.Array(backupCodes), userID)
		if err != nil {
			log.Printf("Database error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not enable 2FA"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success":      true,
			"message":      "2FA enabled successfully",
			"backup_codes": backupCodes,
		})
	}
}

// Disable2FA, kullanıcının 2FA'sını deaktif eder
func Disable2FA(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Kullanıcı kimlik doğrulaması kontrolü
		username, exists := c.Get("username")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		var request struct {
			Password string `json:"password" binding:"required"`
		}

		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}

		// Kullanıcı bilgilerini al ve şifreyi doğrula
		var userID int
		var passwordHash string
		err := db.QueryRow("SELECT id, password_hash FROM users WHERE username = $1", username).Scan(&userID, &passwordHash)
		if err != nil {
			log.Printf("Database error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}

		// Şifre kontrolü
		if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(request.Password)); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid password"})
			return
		}

		// 2FA'yı deaktif et
		_, err = db.Exec("UPDATE users SET totp_enabled = false, totp_secret = NULL, backup_codes = NULL WHERE id = $1", userID)
		if err != nil {
			log.Printf("Database error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not disable 2FA"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "2FA disabled successfully",
		})
	}
}

// Get2FAStatus, kullanıcının 2FA durumunu döndürür
func Get2FAStatus(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Kullanıcı kimlik doğrulaması kontrolü
		username, exists := c.Get("username")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		var totpEnabled bool
		err := db.QueryRow("SELECT COALESCE(totp_enabled, false) FROM users WHERE username = $1", username).Scan(&totpEnabled)
		if err != nil {
			log.Printf("Database error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"enabled": totpEnabled,
		})
	}
}

// Verify2FA, 2FA kodunu doğrular (login sırasında kullanılır)
func Verify2FA(db *sql.DB, username, code string) (bool, error) {
	var totpSecret string
	var backupCodes pq.StringArray

	err := db.QueryRow("SELECT totp_secret, backup_codes FROM users WHERE username = $1 AND totp_enabled = true",
		username).Scan(&totpSecret, &backupCodes)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, fmt.Errorf("2FA not enabled for user")
		}
		return false, err
	}

	// Önce TOTP kodunu kontrol et
	if totp.Validate(code, totpSecret) {
		return true, nil
	}

	// TOTP geçersizse backup kodlarını kontrol et
	for i, backupCode := range backupCodes {
		if backupCode == code {
			// Kullanılan backup kodunu sil
			newBackupCodes := append(backupCodes[:i], backupCodes[i+1:]...)
			_, err := db.Exec("UPDATE users SET backup_codes = $1 WHERE username = $2",
				pq.Array(newBackupCodes), username)
			if err != nil {
				log.Printf("Error updating backup codes: %v", err)
			}
			return true, nil
		}
	}

	return false, nil
}

// generateBackupCodes, backup kodları oluşturur
func generateBackupCodes(count int) []string {
	codes := make([]string, count)
	for i := 0; i < count; i++ {
		codes[i] = generateRandomCode(8)
	}
	return codes
}

// generateRandomCode, rastgele kod oluşturur
func generateRandomCode(length int) string {
	bytes := make([]byte, length)
	rand.Read(bytes)
	return strings.ToUpper(base32.StdEncoding.EncodeToString(bytes)[:length])
}

// RegenerateBackupCodes, yeni backup kodları oluşturur
func RegenerateBackupCodes(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Kullanıcı kimlik doğrulaması kontrolü
		username, exists := c.Get("username")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		var request struct {
			Password string `json:"password" binding:"required"`
		}

		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}

		// Kullanıcı bilgilerini al ve şifreyi doğrula
		var userID int
		var passwordHash string
		var totpEnabled bool
		err := db.QueryRow("SELECT id, password_hash, COALESCE(totp_enabled, false) FROM users WHERE username = $1",
			username).Scan(&userID, &passwordHash, &totpEnabled)
		if err != nil {
			log.Printf("Database error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}

		if !totpEnabled {
			c.JSON(http.StatusBadRequest, gin.H{"error": "2FA is not enabled"})
			return
		}

		// Şifre kontrolü
		if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(request.Password)); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid password"})
			return
		}

		// Yeni backup kodları oluştur
		backupCodes := generateBackupCodes(10)

		// Backup kodları güncelle
		_, err = db.Exec("UPDATE users SET backup_codes = $1 WHERE id = $2",
			pq.Array(backupCodes), userID)
		if err != nil {
			log.Printf("Database error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not regenerate backup codes"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success":      true,
			"message":      "Backup codes regenerated successfully",
			"backup_codes": backupCodes,
		})
	}
}
