package api

import (
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
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
	Email        string    `json:"email"`
	Status       string    `json:"status"`
	Admin        string    `json:"admin"`
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
		var status string
		var admin string
		var email string

		// Sorguyu hazırla - Kullanıcı bilgilerini ve durumunu al
		stmt, err := db.Prepare("SELECT password_hash, email, status, admin FROM users WHERE username = $1")
		if err != nil {
			log.Printf("Prepare error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}
		defer stmt.Close()

		err = stmt.QueryRow(loginRequest.Username).Scan(&passwordHash, &email, &status, &admin)
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

		// Kullanıcı aktif mi kontrol et
		if status != "active" {
			log.Printf("User account not active: %s, status: %s", loginRequest.Username, status)
			c.JSON(http.StatusForbidden, gin.H{"error": "Account is not active"})
			return
		}

		user.Username = loginRequest.Username
		user.PasswordHash = passwordHash
		user.Email = email
		user.Status = status
		user.Admin = admin

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
			"email":    user.Email,
			"admin":    user.Admin,
			"exp":      time.Now().Add(time.Hour * 24).Unix(),
		})

		tokenString, err := token.SignedString(jwtSecretKey)
		if err != nil {
			log.Printf("Token generation error: %v", err)
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

		log.Printf("Login successful for user: %s", user.Username)

		// Frontend'in beklediği formatta yanıt dön
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"user": gin.H{
				"username": user.Username,
				"email":    user.Email,
				"admin":    user.Admin,
			},
			"token": tokenString,
		})
	}
}

// JWT doğrulama middleware'i
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Debug için tüm header'ları logla
		log.Printf("Request headers: %v", c.Request.Header)

		// Token'ı cookie'den al
		tokenString, err := c.Cookie("auth_token")
		if err != nil {
			log.Printf("Cookie token not found: %v", err)

			// Cookie yoksa header'ı kontrol et
			authHeader := c.GetHeader("Authorization")
			log.Printf("Authorization header: %s", authHeader)

			if authHeader != "" && len(authHeader) > 7 {
				tokenString = authHeader[7:] // "Bearer " kısmını çıkar
				log.Printf("Using token from Authorization header")
			}
		} else {
			log.Printf("Using token from cookie")
		}

		if tokenString == "" {
			log.Printf("No token found in request")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization token is required"})
			c.Abort()
			return
		}

		// Token'ı doğrula
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			return jwtSecretKey, nil
		})

		if err != nil {
			log.Printf("Token validation error: %v", err)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}

		if !token.Valid {
			log.Printf("Token is invalid")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		// Token doğruysa, claims'i al
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			log.Printf("Could not get token claims")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token claims"})
			c.Abort()
			return
		}

		// Debug için claims'i logla
		log.Printf("Token claims: %v", claims)

		// Kullanıcı bilgilerini context'e ekle
		c.Set("username", claims["username"])
		c.Set("email", claims["email"])
		c.Set("admin", claims["admin"])

		log.Printf("Auth middleware passed successfully for user: %v", claims["username"])
		c.Next()
	}
}

// CreateUser, yeni bir kullanıcı oluşturur
func CreateUser(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var newUser struct {
			Username string `json:"username" binding:"required"`
			Password string `json:"password" binding:"required"`
			Email    string `json:"email" binding:"required,email"`
			IsAdmin  bool   `json:"is_admin"`
			IsActive string `json:"is_active"`
		}

		if err := c.ShouldBindJSON(&newUser); err != nil {
			log.Printf("Binding error: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}

		// Admin değerini string'e dönüştür
		adminValue := "false"
		if newUser.IsAdmin {
			adminValue = "true"
		}

		// Status değerini kontrol et
		status := "active"
		if newUser.IsActive != "" {
			status = newUser.IsActive
		}

		// Şifreyi hash'le
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newUser.Password), bcrypt.DefaultCost)
		if err != nil {
			log.Printf("Hash error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not hash password"})
			return
		}

		// Prepare statement kullanıyoruz
		stmt, err := db.Prepare("INSERT INTO users (username, password_hash, email, status, admin) VALUES ($1, $2, $3, $4, $5)")
		if err != nil {
			log.Printf("Prepare error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}
		defer stmt.Close()

		// Status varsayılan olarak active
		_, err = stmt.Exec(newUser.Username, string(hashedPassword), newUser.Email, status, adminValue)
		if err != nil {
			log.Printf("Database error: %v", err)
			// PostgreSQL hata detaylarını kontrol et
			if strings.Contains(err.Error(), "duplicate key") {
				if strings.Contains(err.Error(), "username") {
					c.JSON(http.StatusConflict, gin.H{"error": "Username already exists"})
				} else if strings.Contains(err.Error(), "email") {
					c.JSON(http.StatusConflict, gin.H{"error": "Email already exists"})
				} else {
					c.JSON(http.StatusConflict, gin.H{"error": "User already exists"})
				}
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not create user"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "User created successfully",
		})
	}
}

// GetUsers, tüm kullanıcıları listeler
func GetUsers(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Kullanıcıları çek
		rows, err := db.Query("SELECT id, username, email, status, admin, created_at FROM users ORDER BY id")
		if err != nil {
			log.Printf("Database error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Kullanıcılar listelenirken bir hata oluştu",
			})
			return
		}
		defer rows.Close()

		var users []User
		for rows.Next() {
			var user User
			var createdAt time.Time

			if err := rows.Scan(&user.ID, &user.Username, &user.Email, &user.Status, &user.Admin, &createdAt); err != nil {
				log.Printf("Scan error: %v", err)
				continue
			}

			user.CreatedAt = createdAt
			users = append(users, user)
		}

		if err := rows.Err(); err != nil {
			log.Printf("Rows error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Kullanıcılar listelenirken bir hata oluştu",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"users":   users,
		})
	}
}

// UpdateUser, kullanıcı bilgilerini günceller
func UpdateUser(db *sql.DB) gin.HandlerFunc {
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

		userID := c.Param("id")
		if userID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Kullanıcı ID'si gerekli",
			})
			return
		}

		var updateData struct {
			Email  string `json:"email"`
			Status string `json:"status"`
			Admin  string `json:"admin"`
		}

		if err := c.ShouldBindJSON(&updateData); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Geçersiz veri formatı",
			})
			return
		}

		// Status değerini kontrol et
		if updateData.Status != "" && updateData.Status != "active" && updateData.Status != "inactive" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Geçersiz status değeri, 'active' veya 'inactive' olmalı",
			})
			return
		}

		// Admin değerini kontrol et
		if updateData.Admin != "" && updateData.Admin != "true" && updateData.Admin != "false" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Geçersiz admin değeri, 'true' veya 'false' olmalı",
			})
			return
		}

		// Güncelleme sorgusu oluştur
		query := "UPDATE users SET "
		var params []interface{}
		var paramIndex int = 1
		var updates []string

		if updateData.Email != "" {
			updates = append(updates, fmt.Sprintf("email = $%d", paramIndex))
			params = append(params, updateData.Email)
			paramIndex++
		}

		if updateData.Status != "" {
			updates = append(updates, fmt.Sprintf("status = $%d", paramIndex))
			params = append(params, updateData.Status)
			paramIndex++
		}

		if updateData.Admin != "" {
			updates = append(updates, fmt.Sprintf("admin = $%d", paramIndex))
			params = append(params, updateData.Admin)
			paramIndex++
		}

		if len(updates) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Güncellenecek alan belirtilmedi",
			})
			return
		}

		// updated_at alanını güncelle
		updates = append(updates, fmt.Sprintf("updated_at = $%d", paramIndex))
		params = append(params, time.Now())
		paramIndex++

		query += strings.Join(updates, ", ")
		query += fmt.Sprintf(" WHERE id = $%d", paramIndex)
		params = append(params, userID)

		// Güncelleme işlemini gerçekleştir
		result, err := db.Exec(query, params...)
		if err != nil {
			log.Printf("Database error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Kullanıcı güncellenirken bir hata oluştu",
			})
			return
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			log.Printf("Error getting rows affected: %v", err)
		} else if rowsAffected == 0 {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "Kullanıcı bulunamadı",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Kullanıcı başarıyla güncellendi",
		})
	}
}

// DeleteUser, kullanıcıyı siler
func DeleteUser(db *sql.DB) gin.HandlerFunc {
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

		userID := c.Param("id")
		if userID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Kullanıcı ID'si gerekli",
			})
			return
		}

		// Silinecek kullanıcının admin olup olmadığını kontrol et
		var isAdmin string
		err := db.QueryRow("SELECT admin FROM users WHERE id = $1", userID).Scan(&isAdmin)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{
					"success": false,
					"error":   "Kullanıcı bulunamadı",
				})
				return
			}
			log.Printf("Database error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Kullanıcı bilgisi alınırken bir hata oluştu",
			})
			return
		}

		// Son admin kullanıcısını silmeye çalışıyorsa engelle
		if isAdmin == "true" {
			var adminCount int
			err := db.QueryRow("SELECT COUNT(*) FROM users WHERE admin = 'true'").Scan(&adminCount)
			if err != nil {
				log.Printf("Database error: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{
					"success": false,
					"error":   "Admin sayısı kontrol edilirken bir hata oluştu",
				})
				return
			}

			if adminCount <= 1 {
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"error":   "Son admin kullanıcısı silinemez",
				})
				return
			}
		}

		// Kullanıcıyı sil
		result, err := db.Exec("DELETE FROM users WHERE id = $1", userID)
		if err != nil {
			log.Printf("Database error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Kullanıcı silinirken bir hata oluştu",
			})
			return
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			log.Printf("Error getting rows affected: %v", err)
		} else if rowsAffected == 0 {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "Kullanıcı bulunamadı",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Kullanıcı başarıyla silindi",
		})
	}
}
