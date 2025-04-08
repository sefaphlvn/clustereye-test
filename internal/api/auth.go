package api

import (
	"bytes"
	"database/sql"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/bcrypt"
)

// Kullanıcı modeli
type User struct {
	ID           int       `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"` // JSON yanıtında gönderilmeyecek
	CreatedAt    time.Time `json:"created_at"`
}

// JWT için gizli anahtar
var jwtSecretKey = []byte("your-secret-key") // Güvenli bir ortamda saklanmalıdır (env değişkeni veya yapılandırma dosyası)
// Login handler
func Login(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var loginRequest struct {
			Username string `json:"username" binding:"required"`
			Password string `json:"password" binding:"required"`
		}

		// Request body'yi logla
		body, _ := io.ReadAll(c.Request.Body)
		log.Printf("Login request body: %s", string(body))
		c.Request.Body = io.NopCloser(bytes.NewBuffer(body)) // Body'yi geri yükle

		if err := c.ShouldBindJSON(&loginRequest); err != nil {
			log.Printf("Binding error: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}

		// Debug için gelen bilgileri logla
		log.Printf("Login attempt for username: %s", loginRequest.Username)

		var user User
		var passwordHash string

		// Sorguyu hazırla
		stmt, err := db.Prepare("SELECT password_hash FROM users WHERE username = $1")
		if err != nil {
			log.Printf("Prepare error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}
		defer stmt.Close()

		err = stmt.QueryRow(loginRequest.Username).Scan(&passwordHash)
		if err != nil {
			if err == sql.ErrNoRows {
				log.Printf("User not found: %s", loginRequest.Username)
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid username or password"})
				return
			}
			log.Printf("Database error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}

		user.Username = loginRequest.Username
		user.PasswordHash = passwordHash

		// Debug için hash'leri logla
		log.Printf("Stored hash: %s", user.PasswordHash)
		log.Printf("Attempting to compare with password: %s", loginRequest.Password)

		// Şifre kontrolü
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(loginRequest.Password)); err != nil {
			log.Printf("Password comparison failed: %v", err)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid username or password"})
			return
		}

		// JWT token oluştur
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"username": user.Username,
			"exp":      time.Now().Add(time.Hour * 24).Unix(),
		})

		tokenString, err := token.SignedString(jwtSecretKey)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not generate token"})
			return
		}

		// Cookie olarak token'ı ayarla
		c.SetCookie(
			"auth_token",
			tokenString,
			3600*24, // 24 saat
			"/",
			"",
			false,
			true,
		)

		// Frontend'in beklediği formatta yanıt dön
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"user": gin.H{
				"username": user.Username,
			},
			"token": tokenString,
		})
	}
}

// JWT doğrulama middleware'i
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Token'ı cookie'den al
		tokenString, err := c.Cookie("auth_token")
		if err != nil {
			// Cookie yoksa header'ı kontrol et
			authHeader := c.GetHeader("Authorization")
			if authHeader != "" && len(authHeader) > 7 {
				tokenString = authHeader[7:] // "Bearer " kısmını çıkar
			}
		}

		if tokenString == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization token is required"})
			c.Abort()
			return
		}

		// Token'ı doğrula
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			return jwtSecretKey, nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}

		// Token doğruysa, claims'i al
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token claims"})
			c.Abort()
			return
		}

		// Kullanıcı adını context'e ekle
		c.Set("username", claims["username"])
		c.Next()
	}
}
