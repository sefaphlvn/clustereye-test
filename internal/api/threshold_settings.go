package api

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
)

type ThresholdSettings struct {
	ID                      int     `json:"id"`
	CPUThreshold            float64 `json:"cpu_threshold"`
	MemoryThreshold         float64 `json:"memory_threshold"`
	DiskThreshold           float64 `json:"disk_threshold"`
	SlowQueryThreshold      int64   `json:"slow_query_threshold_ms"`
	ConnectionThreshold     int     `json:"connection_threshold"`
	ReplicationLagThreshold int     `json:"replication_lag_threshold"`
}

// GetThresholdSettings, threshold ayarlarını veritabanından getirir
func GetThresholdSettings(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var settings ThresholdSettings
		err := db.QueryRow("SELECT id, cpu_threshold, memory_threshold, disk_threshold, slow_query_threshold_ms, connection_threshold, replication_lag_threshold FROM threshold_settings ORDER BY id DESC LIMIT 1").
			Scan(&settings.ID, &settings.CPUThreshold, &settings.MemoryThreshold, &settings.DiskThreshold, &settings.SlowQueryThreshold, &settings.ConnectionThreshold, &settings.ReplicationLagThreshold)

		if err != nil {
			if err == sql.ErrNoRows {
				// Eğer ayar bulunamazsa varsayılan değerleri döndür
				settings = ThresholdSettings{
					CPUThreshold:            80.0, // %80
					MemoryThreshold:         80.0, // %80
					DiskThreshold:           85.0, // %85
					SlowQueryThreshold:      1000, // 1 saniye
					ConnectionThreshold:     100,  // 100 bağlantı
					ReplicationLagThreshold: 300,  // 300 saniye
				}
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{
					"status": "error",
					"error":  "Threshold ayarları alınamadı: " + err.Error(),
				})
				return
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   settings,
		})
	}
}

// UpdateThresholdSettings, threshold ayarlarını günceller
func UpdateThresholdSettings(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var settings ThresholdSettings
		if err := c.ShouldBindJSON(&settings); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Geçersiz istek formatı: " + err.Error(),
			})
			return
		}

		// Değerlerin geçerliliğini kontrol et
		if settings.CPUThreshold < 0 || settings.CPUThreshold > 100 ||
			settings.MemoryThreshold < 0 || settings.MemoryThreshold > 100 ||
			settings.DiskThreshold < 0 || settings.DiskThreshold > 100 ||
			settings.SlowQueryThreshold < 0 ||
			settings.ConnectionThreshold < 0 ||
			settings.ReplicationLagThreshold < 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Geçersiz threshold değerleri",
			})
			return
		}

		// Veritabanında güncelle veya yeni kayıt ekle
		result, err := db.Exec(`
			INSERT INTO threshold_settings 
			(cpu_threshold, memory_threshold, disk_threshold, slow_query_threshold_ms, connection_threshold, replication_lag_threshold)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			settings.CPUThreshold,
			settings.MemoryThreshold,
			settings.DiskThreshold,
			settings.SlowQueryThreshold,
			settings.ConnectionThreshold,
			settings.ReplicationLagThreshold,
		)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "Threshold ayarları güncellenemedi: " + err.Error(),
			})
			return
		}

		id, _ := result.LastInsertId()
		settings.ID = int(id)

		c.JSON(http.StatusOK, gin.H{
			"status":  "success",
			"message": "Threshold ayarları başarıyla güncellendi",
			"data":    settings,
		})
	}
}
