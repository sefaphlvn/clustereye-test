package api

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sefaphlvn/clustereye-test/internal/server"
)

// PostgreSQL System Metrics Handlers

// fixAgentID, agent_id formatını düzeltir (önüne "agent_" ekler)
func fixAgentID(agentID string) string {
	if agentID == "" {
		return ""
	}
	if !strings.HasPrefix(agentID, "agent_") {
		return "agent_" + agentID
	}
	return agentID
}

// getPostgreSQLSystemCPUMetrics, PostgreSQL sistem CPU metriklerini getirir
func getPostgreSQLSystemCPUMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		timeRange := c.DefaultQuery("range", "1h")
		fullAgentID := fixAgentID(agentID)

		var query string
		if fullAgentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_system") |> filter(fn: (r) => r._field == "cpu_usage" or r._field == "cpu_cores") |> filter(fn: (r) => r.agent_id == "%s") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, fullAgentID)
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_system") |> filter(fn: (r) => r._field == "cpu_usage" or r._field == "cpu_cores") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		influxWriter := server.GetInfluxWriter()
		if influxWriter == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "error",
				"error":  "InfluxDB servisi kullanılamıyor",
			})
			return
		}

		results, err := influxWriter.QueryMetrics(ctx, query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "PostgreSQL sistem CPU metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getPostgreSQLSystemMemoryMetrics, PostgreSQL sistem memory metriklerini getirir
func getPostgreSQLSystemMemoryMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		timeRange := c.DefaultQuery("range", "1h")
		fullAgentID := fixAgentID(agentID)

		var query string
		if fullAgentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_system") |> filter(fn: (r) => r._field == "free_memory" or r._field == "total_memory") |> filter(fn: (r) => r.agent_id == "%s") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, fullAgentID)
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_system") |> filter(fn: (r) => r._field == "free_memory" or r._field == "total_memory") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		influxWriter := server.GetInfluxWriter()
		if influxWriter == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "error",
				"error":  "InfluxDB servisi kullanılamıyor",
			})
			return
		}

		results, err := influxWriter.QueryMetrics(ctx, query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "PostgreSQL sistem memory metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getPostgreSQLSystemDiskMetrics, PostgreSQL sistem disk metriklerini getirir
func getPostgreSQLSystemDiskMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		timeRange := c.DefaultQuery("range", "1h")
		fullAgentID := fixAgentID(agentID)

		var query string
		if fullAgentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_system") |> filter(fn: (r) => r._field == "free_disk" or r._field == "total_disk") |> filter(fn: (r) => r.agent_id == "%s") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, fullAgentID)
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_system") |> filter(fn: (r) => r._field == "free_disk" or r._field == "total_disk") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		influxWriter := server.GetInfluxWriter()
		if influxWriter == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "error",
				"error":  "InfluxDB servisi kullanılamıyor",
			})
			return
		}

		results, err := influxWriter.QueryMetrics(ctx, query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "PostgreSQL sistem disk metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getPostgreSQLSystemResponseTimeMetrics, PostgreSQL sistem response time metriklerini getirir
func getPostgreSQLSystemResponseTimeMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		timeRange := c.DefaultQuery("range", "1h")
		fullAgentID := fixAgentID(agentID)

		var query string
		if fullAgentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_system") |> filter(fn: (r) => r._field == "response_time_ms") |> filter(fn: (r) => r.agent_id == "%s") |> aggregateWindow(every: 1m, fn: mean, createEmpty: false) |> sort(columns: ["_time"], desc: false)`, timeRange, fullAgentID)
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_system") |> filter(fn: (r) => r._field == "response_time_ms") |> aggregateWindow(every: 1m, fn: mean, createEmpty: false) |> sort(columns: ["_time"], desc: false)`, timeRange)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		influxWriter := server.GetInfluxWriter()
		if influxWriter == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "error",
				"error":  "InfluxDB servisi kullanılamıyor",
			})
			return
		}

		results, err := influxWriter.QueryMetrics(ctx, query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "PostgreSQL response time metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		// Response time istatistikleri hesapla
		var totalResponseTime, minResponseTime, maxResponseTime, avgResponseTime float64
		var responseTimeCount int

		if len(results) > 0 {
			minResponseTime = 99999999 // Büyük bir başlangıç değeri
			maxResponseTime = 0

			for _, result := range results {
				if val, ok := result["_value"]; ok {
					if responseTime, ok := val.(float64); ok && responseTime > 0 {
						totalResponseTime += responseTime
						responseTimeCount++

						if responseTime < minResponseTime {
							minResponseTime = responseTime
						}
						if responseTime > maxResponseTime {
							maxResponseTime = responseTime
						}
					}
				}
			}

			if responseTimeCount > 0 {
				avgResponseTime = totalResponseTime / float64(responseTimeCount)
			}

			// Eğer hiç veri yoksa minimum değeri sıfırlayalım
			if responseTimeCount == 0 {
				minResponseTime = 0
			}
		}

		// Latest response time'ı al
		var latestResponseTime float64
		var latestTimestamp string
		if len(results) > 0 {
			// Son kayıttan en güncel değeri al
			lastResult := results[len(results)-1]
			if val, ok := lastResult["_value"]; ok {
				if responseTime, ok := val.(float64); ok {
					latestResponseTime = responseTime
				}
			}
			if timestamp, ok := lastResult["_time"]; ok {
				if timeStr, ok := timestamp.(string); ok {
					latestTimestamp = timeStr
				}
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data": gin.H{
				"agent_id": agentID,
				"summary": gin.H{
					"latest_response_time_ms": latestResponseTime,
					"latest_timestamp":        latestTimestamp,
					"avg_response_time_ms":    avgResponseTime,
					"min_response_time_ms":    minResponseTime,
					"max_response_time_ms":    maxResponseTime,
					"total_measurements":      responseTimeCount,
				},
				"all_data": results,
			},
		})
	}
}

// PostgreSQL Database Metrics Handlers

// getPostgreSQLConnectionsMetrics, PostgreSQL bağlantı metriklerini getirir
func getPostgreSQLConnectionsMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		timeRange := c.DefaultQuery("range", "1h")

		// Agent ID formatını düzelt - önüne "agent_" ekle
		var fullAgentID string
		if agentID != "" {
			if !strings.HasPrefix(agentID, "agent_") {
				fullAgentID = "agent_" + agentID
			} else {
				fullAgentID = agentID
			}
		}

		// Tüm PostgreSQL connection field'larını dahil et
		connectionFields := `r._field == "active" or r._field == "available" or r._field == "avg_age_seconds" or r._field == "blocked" or r._field == "blocking" or r._field == "by_application" or r._field == "by_database" or r._field == "conflicts_total" or r._field == "effective_max_connections" or r._field == "idle" or r._field == "idle_in_transaction" or r._field == "idle_in_transaction_aborted" or r._field == "long_running_queries" or r._field == "max_connections" or r._field == "middle_age_connections" or r._field == "old_connections" or r._field == "new_connections" or r._field == "oldest_connection_seconds" or r._field == "reuse_ratio" or r._field == "superuser_reserved" or r._field == "total" or r._field == "temp_byres_used" or r._field == "temp_files_created" or r._field == "utilization_percent" or r._field == "waiting_on_io" or r._field == "waiting_on_locks" or r._field == "waiting_on_lwlocks" or r._field == "waiting_total" or r._field == "young_connections"`

		var query string
		if fullAgentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_connections") |> filter(fn: (r) => %s) |> filter(fn: (r) => r.agent_id == "%s") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, connectionFields, fullAgentID)
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_connections") |> filter(fn: (r) => %s) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, connectionFields)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		influxWriter := server.GetInfluxWriter()
		if influxWriter == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "error",
				"error":  "InfluxDB servisi kullanılamıyor",
			})
			return
		}

		results, err := influxWriter.QueryMetrics(ctx, query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "PostgreSQL bağlantı metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		// DEBUG bilgilerini kaldır, normal response dön
		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getPostgreSQLTransactionsMetrics, PostgreSQL transaction metriklerini getirir
func getPostgreSQLTransactionsMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		database := c.Query("database") // İsteğe bağlı database filtresi
		timeRange := c.DefaultQuery("range", "1h")
		fullAgentID := fixAgentID(agentID)

		var query string
		if fullAgentID != "" && database != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_database") |> filter(fn: (r) => r._field == "xact_commit" or r._field == "xact_rollback") |> filter(fn: (r) => r.agent_id == "%s") |> filter(fn: (r) => r.database =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, fullAgentID, regexp.QuoteMeta(database))
		} else if fullAgentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_database") |> filter(fn: (r) => r._field == "xact_commit" or r._field == "xact_rollback") |> filter(fn: (r) => r.agent_id == "%s") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, fullAgentID)
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_database") |> filter(fn: (r) => r._field == "xact_commit" or r._field == "xact_rollback") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		influxWriter := server.GetInfluxWriter()
		if influxWriter == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "error",
				"error":  "InfluxDB servisi kullanılamıyor",
			})
			return
		}

		results, err := influxWriter.QueryMetrics(ctx, query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "PostgreSQL transaction metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getPostgreSQLCacheMetrics, PostgreSQL cache performance metriklerini getirir
func getPostgreSQLCacheMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		database := c.Query("database") // İsteğe bağlı database filtresi
		timeRange := c.DefaultQuery("range", "1h")
		fullAgentID := fixAgentID(agentID)

		var query string
		if fullAgentID != "" && database != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_database") |> filter(fn: (r) => r._field == "blks_hit" or r._field == "blks_read") |> filter(fn: (r) => r.agent_id == "%s") |> filter(fn: (r) => r.database =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, fullAgentID, regexp.QuoteMeta(database))
		} else if fullAgentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_database") |> filter(fn: (r) => r._field == "blks_hit" or r._field == "blks_read") |> filter(fn: (r) => r.agent_id == "%s") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, fullAgentID)
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_database") |> filter(fn: (r) => r._field == "blks_hit" or r._field == "blks_read") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		influxWriter := server.GetInfluxWriter()
		if influxWriter == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "error",
				"error":  "InfluxDB servisi kullanılamıyor",
			})
			return
		}

		results, err := influxWriter.QueryMetrics(ctx, query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "PostgreSQL cache metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		// Cache hit ratio hesapla
		var totalCacheHitRatio float64
		var cacheHitRatioCount int
		var latestCacheHitRatio float64

		// Basit cache hit ratio hesaplaması için blks_hit ve blks_read değerlerini kullan
		blksHitData := make(map[string]float64)
		blksReadData := make(map[string]float64)

		for _, result := range results {
			if timestamp, ok := result["_time"].(string); ok {
				if field, ok := result["_field"].(string); ok {
					// Değeri float64'e dönüştür (int64, float64, string olabilir)
					var value float64
					switch v := result["_value"].(type) {
					case float64:
						value = v
					case int64:
						value = float64(v)
					case int:
						value = float64(v)
					case string:
						if parsed, err := strconv.ParseFloat(v, 64); err == nil {
							value = parsed
						} else {
							continue // Bu record'u atla
						}
					default:
						continue // Bu record'u atla
					}

					if field == "blks_hit" {
						blksHitData[timestamp] = value
					} else if field == "blks_read" {
						blksReadData[timestamp] = value
					}
				}
			}
		}

		// Cache hit ratio'yu hesapla
		for timestamp, blksHit := range blksHitData {
			if blksRead, exists := blksReadData[timestamp]; exists {
				totalBlocks := blksHit + blksRead
				if totalBlocks > 0 {
					hitRatio := (blksHit / totalBlocks) * 100
					totalCacheHitRatio += hitRatio
					cacheHitRatioCount++
					latestCacheHitRatio = hitRatio // En son hesaplanan değer
				}
			}
		}

		var avgCacheHitRatio float64
		if cacheHitRatioCount > 0 {
			avgCacheHitRatio = totalCacheHitRatio / float64(cacheHitRatioCount)
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data": gin.H{
				"raw_data": results,
				"summary": gin.H{
					"latest_cache_hit_ratio_percent": latestCacheHitRatio,
					"avg_cache_hit_ratio_percent":    avgCacheHitRatio,
					"total_measurements":             cacheHitRatioCount,
					"debug_info": gin.H{
						"blks_hit_records":  len(blksHitData),
						"blks_read_records": len(blksReadData),
						"total_raw_records": len(results),
					},
				},
			},
		})
	}
}

// getPostgreSQLDeadlocksMetrics, PostgreSQL deadlock metriklerini getirir
func getPostgreSQLDeadlocksMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		database := c.Query("database") // İsteğe bağlı database filtresi
		timeRange := c.DefaultQuery("range", "1h")
		fullAgentID := fixAgentID(agentID)

		var query string
		if fullAgentID != "" && database != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_database") |> filter(fn: (r) => r._field == "deadlocks") |> filter(fn: (r) => r.agent_id == "%s") |> filter(fn: (r) => r.database =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, fullAgentID, regexp.QuoteMeta(database))
		} else if fullAgentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_database") |> filter(fn: (r) => r._field == "deadlocks") |> filter(fn: (r) => r.agent_id == "%s") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, fullAgentID)
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_database") |> filter(fn: (r) => r._field == "deadlocks") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		influxWriter := server.GetInfluxWriter()
		if influxWriter == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "error",
				"error":  "InfluxDB servisi kullanılamıyor",
			})
			return
		}

		results, err := influxWriter.QueryMetrics(ctx, query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "PostgreSQL deadlock metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getPostgreSQLReplicationMetrics, PostgreSQL replication lag metriklerini getirir
func getPostgreSQLReplicationMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		timeRange := c.DefaultQuery("range", "1h")
		fullAgentID := fixAgentID(agentID)

		// Replication lag için özel measurement olabileceğini varsayıyoruz
		// Eğer mevcut değilse postgresql_system'de replication_lag field'ı olabilir
		var query string
		if fullAgentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_replication" or (r._measurement == "postgresql_system" and r._field == "replication_lag")) |> filter(fn: (r) => r.agent_id == "%s")`, timeRange, fullAgentID)
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_replication" or (r._measurement == "postgresql_system" and r._field == "replication_lag"))`, timeRange)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		influxWriter := server.GetInfluxWriter()
		if influxWriter == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "error",
				"error":  "InfluxDB servisi kullanılamıyor",
			})
			return
		}

		results, err := influxWriter.QueryMetrics(ctx, query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "PostgreSQL replication metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getPostgreSQLLocksMetrics, PostgreSQL lock metriklerini getirir
func getPostgreSQLLocksMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		lockType := c.Query("lock_type") // İsteğe bağlı lock type filtresi
		timeRange := c.DefaultQuery("range", "1h")
		fullAgentID := fixAgentID(agentID)

		var query string
		if fullAgentID != "" && lockType != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_locks") |> filter(fn: (r) => r._field == "granted" or r._field == "waiting") |> filter(fn: (r) => r.agent_id == "%s") |> filter(fn: (r) => r.lock_type =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, fullAgentID, regexp.QuoteMeta(lockType))
		} else if fullAgentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_locks") |> filter(fn: (r) => r._field == "granted" or r._field == "waiting") |> filter(fn: (r) => r.agent_id == "%s") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, fullAgentID)
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_locks") |> filter(fn: (r) => r._field == "granted" or r._field == "waiting") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		influxWriter := server.GetInfluxWriter()
		if influxWriter == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "error",
				"error":  "InfluxDB servisi kullanılamıyor",
			})
			return
		}

		results, err := influxWriter.QueryMetrics(ctx, query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "PostgreSQL lock metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getPostgreSQLTablesMetrics, PostgreSQL table performance metriklerini getirir
func getPostgreSQLTablesMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		database := c.Query("database") // İsteğe bağlı database filtresi
		tableName := c.Query("table")   // İsteğe bağlı table filtresi
		timeRange := c.DefaultQuery("range", "1h")
		limit := c.DefaultQuery("limit", "100") // Tablo sayısı limiti
		fullAgentID := fixAgentID(agentID)

		// Limit'i integer'a çevir
		limitInt, err := strconv.Atoi(limit)
		if err != nil || limitInt <= 0 {
			limitInt = 100
		}

		// PostgreSQL table metrikleri için özel measurement
		var query string
		if fullAgentID != "" && database != "" && tableName != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_tables") |> filter(fn: (r) => r.agent_id == "%s") |> filter(fn: (r) => r.database =~ /^%s$/) |> filter(fn: (r) => r.table =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false) |> limit(n: %d)`, timeRange, fullAgentID, regexp.QuoteMeta(database), regexp.QuoteMeta(tableName), limitInt)
		} else if fullAgentID != "" && database != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_tables") |> filter(fn: (r) => r.agent_id == "%s") |> filter(fn: (r) => r.database =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false) |> limit(n: %d)`, timeRange, fullAgentID, regexp.QuoteMeta(database), limitInt)
		} else if fullAgentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_tables") |> filter(fn: (r) => r.agent_id == "%s") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false) |> limit(n: %d)`, timeRange, fullAgentID, limitInt)
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_tables") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false) |> limit(n: %d)`, timeRange, limitInt)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		influxWriter := server.GetInfluxWriter()
		if influxWriter == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "error",
				"error":  "InfluxDB servisi kullanılamıyor",
			})
			return
		}

		results, err := influxWriter.QueryMetrics(ctx, query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "PostgreSQL table metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getPostgreSQLIndexesMetrics, PostgreSQL index performance metriklerini getirir
func getPostgreSQLIndexesMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		database := c.Query("database") // İsteğe bağlı database filtresi
		indexName := c.Query("index")   // İsteğe bağlı index filtresi
		timeRange := c.DefaultQuery("range", "1h")
		limit := c.DefaultQuery("limit", "100") // Index sayısı limiti
		fullAgentID := fixAgentID(agentID)

		// Limit'i integer'a çevir
		limitInt, err := strconv.Atoi(limit)
		if err != nil || limitInt <= 0 {
			limitInt = 100
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		influxWriter := server.GetInfluxWriter()
		if influxWriter == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "error",
				"error":  "InfluxDB servisi kullanılamıyor",
			})
			return
		}

		// Group by kullanarak her alan için en son değeri al
		var query string
		if fullAgentID != "" && database != "" && indexName != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") 
				|> range(start: -%s) 
				|> filter(fn: (r) => r._measurement == "postgresql_index") 
				|> filter(fn: (r) => r.agent_id == "%s") 
				|> filter(fn: (r) => r.database =~ /^%s$/) 
				|> filter(fn: (r) => r.index =~ /^%s$/)
				|> group(columns: ["_field", "index", "database", "agent_id"])
				|> last()
				|> limit(n: %d)`,
				timeRange, fullAgentID, regexp.QuoteMeta(database), regexp.QuoteMeta(indexName), limitInt)
		} else if fullAgentID != "" && database != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") 
				|> range(start: -%s) 
				|> filter(fn: (r) => r._measurement == "postgresql_index") 
				|> filter(fn: (r) => r.agent_id == "%s") 
				|> filter(fn: (r) => r.database =~ /^%s$/)
				|> group(columns: ["_field", "index", "database", "agent_id"])
				|> last()
				|> limit(n: %d)`,
				timeRange, fullAgentID, regexp.QuoteMeta(database), limitInt)
		} else if fullAgentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") 
				|> range(start: -%s) 
				|> filter(fn: (r) => r._measurement == "postgresql_index") 
				|> filter(fn: (r) => r.agent_id == "%s")
				|> group(columns: ["_field", "index", "database", "agent_id"])
				|> last()
				|> limit(n: %d)`,
				timeRange, fullAgentID, limitInt)
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") 
				|> range(start: -%s) 
				|> filter(fn: (r) => r._measurement == "postgresql_index")
				|> group(columns: ["_field", "index", "database", "agent_id"])
				|> last()
				|> limit(n: %d)`,
				timeRange, limitInt)
		}

		results, err := influxWriter.QueryMetrics(ctx, query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "PostgreSQL index metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// PostgreSQL Performance Metrics Handlers

// getPostgreSQLPerformanceQPSMetrics, PostgreSQL QPS (queries per second) metriklerini getirir
func getPostgreSQLPerformanceQPSMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		database := c.Query("database")
		timeRange := c.DefaultQuery("range", "1h")
		fullAgentID := fixAgentID(agentID)

		var query string
		if fullAgentID != "" && database != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_performance") |> filter(fn: (r) => r._field == "queries_per_sec") |> filter(fn: (r) => r.agent_id == "%s") |> filter(fn: (r) => r.database =~ /^%s$/) |> aggregateWindow(every: 1m, fn: mean, createEmpty: false)`, timeRange, fullAgentID, regexp.QuoteMeta(database))
		} else if fullAgentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_performance") |> filter(fn: (r) => r._field == "queries_per_sec") |> filter(fn: (r) => r.agent_id == "%s") |> aggregateWindow(every: 1m, fn: mean, createEmpty: false)`, timeRange, fullAgentID)
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_performance") |> filter(fn: (r) => r._field == "queries_per_sec") |> aggregateWindow(every: 1m, fn: mean, createEmpty: false)`, timeRange)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		influxWriter := server.GetInfluxWriter()
		if influxWriter == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "error",
				"error":  "InfluxDB servisi kullanılamıyor",
			})
			return
		}

		results, err := influxWriter.QueryMetrics(ctx, query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "PostgreSQL QPS metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getPostgreSQLPerformanceQueryTimeMetrics, PostgreSQL query süresi metriklerini getirir
func getPostgreSQLPerformanceQueryTimeMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		database := c.Query("database")
		timeRange := c.DefaultQuery("range", "1h")
		fullAgentID := fixAgentID(agentID)

		var query string
		queryTimeFields := "r._field == \"avg_query_time_ms\" or r._field == \"query_time_p95_ms\" or r._field == \"query_time_p99_ms\""

		if fullAgentID != "" && database != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_performance") |> filter(fn: (r) => %s) |> filter(fn: (r) => r.agent_id == "%s") |> filter(fn: (r) => r.database =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, queryTimeFields, fullAgentID, regexp.QuoteMeta(database))
		} else if fullAgentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_performance") |> filter(fn: (r) => %s) |> filter(fn: (r) => r.agent_id == "%s") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, queryTimeFields, fullAgentID)
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_performance") |> filter(fn: (r) => %s) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, queryTimeFields)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		influxWriter := server.GetInfluxWriter()
		if influxWriter == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "error",
				"error":  "InfluxDB servisi kullanılamıyor",
			})
			return
		}

		results, err := influxWriter.QueryMetrics(ctx, query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "PostgreSQL query süresi metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getPostgreSQLPerformanceSlowQueriesMetrics, PostgreSQL yavaş query metriklerini getirir
func getPostgreSQLPerformanceSlowQueriesMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		database := c.Query("database")
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if agentID != "" && database != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_performance") |> filter(fn: (r) => r._field == "slow_queries_count") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> filter(fn: (r) => r.database =~ /^%s$/) |> aggregateWindow(every: 5m, fn: sum, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID), regexp.QuoteMeta(database))
		} else if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_performance") |> filter(fn: (r) => r._field == "slow_queries_count") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: sum, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_performance") |> filter(fn: (r) => r._field == "slow_queries_count") |> aggregateWindow(every: 5m, fn: sum, createEmpty: false)`, timeRange)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		influxWriter := server.GetInfluxWriter()
		if influxWriter == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "error",
				"error":  "InfluxDB servisi kullanılamıyor",
			})
			return
		}

		results, err := influxWriter.QueryMetrics(ctx, query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "PostgreSQL yavaş query metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getPostgreSQLPerformanceActiveQueriesMetrics, PostgreSQL aktif query metriklerini getirir
func getPostgreSQLPerformanceActiveQueriesMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		database := c.Query("database")
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if agentID != "" && database != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_performance") |> filter(fn: (r) => r._field == "active_queries_count") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> filter(fn: (r) => r.database =~ /^%s$/) |> aggregateWindow(every: 1m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID), regexp.QuoteMeta(database))
		} else if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_performance") |> filter(fn: (r) => r._field == "active_queries_count") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 1m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_performance") |> filter(fn: (r) => r._field == "active_queries_count") |> aggregateWindow(every: 1m, fn: mean, createEmpty: false)`, timeRange)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		influxWriter := server.GetInfluxWriter()
		if influxWriter == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "error",
				"error":  "InfluxDB servisi kullanılamıyor",
			})
			return
		}

		results, err := influxWriter.QueryMetrics(ctx, query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "PostgreSQL aktif query metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getPostgreSQLPerformanceLongRunningQueriesMetrics, PostgreSQL uzun süren query metriklerini getirir
func getPostgreSQLPerformanceLongRunningQueriesMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		database := c.Query("database")
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if agentID != "" && database != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_performance") |> filter(fn: (r) => r._field == "long_running_queries_count") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> filter(fn: (r) => r.database =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID), regexp.QuoteMeta(database))
		} else if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_performance") |> filter(fn: (r) => r._field == "long_running_queries_count") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_performance") |> filter(fn: (r) => r._field == "long_running_queries_count") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		influxWriter := server.GetInfluxWriter()
		if influxWriter == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "error",
				"error":  "InfluxDB servisi kullanılamıyor",
			})
			return
		}

		results, err := influxWriter.QueryMetrics(ctx, query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "PostgreSQL uzun süren query metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getPostgreSQLPerformanceReadWriteRatioMetrics, PostgreSQL Read/Write oranı metriklerini getirir
func getPostgreSQLPerformanceReadWriteRatioMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		database := c.Query("database")
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if agentID != "" && database != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_performance") |> filter(fn: (r) => r._field == "read_write_ratio") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> filter(fn: (r) => r.database =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID), regexp.QuoteMeta(database))
		} else if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_performance") |> filter(fn: (r) => r._field == "read_write_ratio") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_performance") |> filter(fn: (r) => r._field == "read_write_ratio") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		influxWriter := server.GetInfluxWriter()
		if influxWriter == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "error",
				"error":  "InfluxDB servisi kullanılamıyor",
			})
			return
		}

		results, err := influxWriter.QueryMetrics(ctx, query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "PostgreSQL Read/Write oranı metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getPostgreSQLPerformanceIndexScanRatioMetrics, PostgreSQL index kullanım oranı metriklerini getirir
func getPostgreSQLPerformanceIndexScanRatioMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		database := c.Query("database")
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if agentID != "" && database != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_performance") |> filter(fn: (r) => r._field == "index_scan_ratio") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> filter(fn: (r) => r.database =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID), regexp.QuoteMeta(database))
		} else if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_performance") |> filter(fn: (r) => r._field == "index_scan_ratio") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_performance") |> filter(fn: (r) => r._field == "index_scan_ratio") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		influxWriter := server.GetInfluxWriter()
		if influxWriter == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "error",
				"error":  "InfluxDB servisi kullanılamıyor",
			})
			return
		}

		results, err := influxWriter.QueryMetrics(ctx, query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "PostgreSQL index kullanım oranı metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getPostgreSQLPerformanceSeqScanRatioMetrics, PostgreSQL sequential scan oranı metriklerini getirir
func getPostgreSQLPerformanceSeqScanRatioMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		database := c.Query("database")
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if agentID != "" && database != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_performance") |> filter(fn: (r) => r._field == "seq_scan_ratio") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> filter(fn: (r) => r.database =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID), regexp.QuoteMeta(database))
		} else if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_performance") |> filter(fn: (r) => r._field == "seq_scan_ratio") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_performance") |> filter(fn: (r) => r._field == "seq_scan_ratio") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		influxWriter := server.GetInfluxWriter()
		if influxWriter == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "error",
				"error":  "InfluxDB servisi kullanılamıyor",
			})
			return
		}

		results, err := influxWriter.QueryMetrics(ctx, query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "PostgreSQL sequential scan oranı metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getPostgreSQLPerformanceCacheHitRatioMetrics, PostgreSQL cache hit oranı metriklerini getirir
func getPostgreSQLPerformanceCacheHitRatioMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		database := c.Query("database")
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if agentID != "" && database != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_performance") |> filter(fn: (r) => r._field == "cache_hit_ratio") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> filter(fn: (r) => r.database =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID), regexp.QuoteMeta(database))
		} else if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_performance") |> filter(fn: (r) => r._field == "cache_hit_ratio") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_performance") |> filter(fn: (r) => r._field == "cache_hit_ratio") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		influxWriter := server.GetInfluxWriter()
		if influxWriter == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "error",
				"error":  "InfluxDB servisi kullanılamıyor",
			})
			return
		}

		results, err := influxWriter.QueryMetrics(ctx, query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "PostgreSQL cache hit oranı metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getPostgreSQLPerformanceAllMetrics, tüm PostgreSQL performance metriklerini tek seferde getirir
func getPostgreSQLPerformanceAllMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		database := c.Query("database")
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if agentID != "" && database != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_performance") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> filter(fn: (r) => r.database =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID), regexp.QuoteMeta(database))
		} else if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_performance") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_performance") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		influxWriter := server.GetInfluxWriter()
		if influxWriter == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "error",
				"error":  "InfluxDB servisi kullanılamıyor",
			})
			return
		}

		results, err := influxWriter.QueryMetrics(ctx, query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "PostgreSQL performance metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getPostgreSQLActiveQueriesDetails, PostgreSQL aktif query detaylarını getirir
func getPostgreSQLActiveQueriesDetails(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		database := c.Query("database")
		timeRange := c.DefaultQuery("range", "10m") // Varsayılan olarak son 10 dakika
		limit := c.DefaultQuery("limit", "50")      // Varsayılan olarak en fazla 50 sorgu
		fullAgentID := fixAgentID(agentID)

		var query string
		if fullAgentID != "" && database != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_active_query") |> filter(fn: (r) => r.agent_id == "%s") |> filter(fn: (r) => r.database == "%s") |> sort(columns: ["_time"], desc: true) |> limit(n: %s)`, timeRange, fullAgentID, database, limit)
		} else if fullAgentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_active_query") |> filter(fn: (r) => r.agent_id == "%s") |> sort(columns: ["_time"], desc: true) |> limit(n: %s)`, timeRange, fullAgentID, limit)
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_active_query") |> sort(columns: ["_time"], desc: true) |> limit(n: %s)`, timeRange, limit)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		influxWriter := server.GetInfluxWriter()
		if influxWriter == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "error",
				"error":  "InfluxDB servisi kullanılamıyor",
			})
			return
		}

		results, err := influxWriter.QueryMetrics(ctx, query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "PostgreSQL aktif query detayları alınamadı: " + err.Error(),
			})
			return
		}

		// Sonuçları daha kullanışlı bir formata dönüştür
		type ActiveQuery struct {
			QueryText       string    `json:"query_text"`
			DurationSeconds float64   `json:"duration_seconds"`
			Database        string    `json:"database"`
			Username        string    `json:"username"`
			Application     string    `json:"application"`
			State           string    `json:"state"`
			ClientAddr      string    `json:"client_addr,omitempty"`
			WaitEventType   string    `json:"wait_event_type,omitempty"`
			WaitEvent       string    `json:"wait_event,omitempty"`
			Timestamp       time.Time `json:"timestamp"`
		}

		activeQueries := make([]ActiveQuery, 0)
		for _, result := range results {
			var query ActiveQuery

			// Timestamp
			if timeValue, ok := result["_time"].(time.Time); ok {
				query.Timestamp = timeValue
			}

			// Fields
			if queryText, ok := result["query_text"].(string); ok {
				query.QueryText = queryText
			}
			if duration, ok := result["duration_seconds"].(float64); ok {
				query.DurationSeconds = duration
			}

			// Tags
			if db, ok := result["database"].(string); ok {
				query.Database = db
			}
			if username, ok := result["username"].(string); ok {
				query.Username = username
			}
			if app, ok := result["application"].(string); ok {
				query.Application = app
			}
			if state, ok := result["state"].(string); ok {
				query.State = state
			}
			if clientAddr, ok := result["client_addr"].(string); ok {
				query.ClientAddr = clientAddr
			}
			if waitEventType, ok := result["wait_event_type"].(string); ok {
				query.WaitEventType = waitEventType
			}
			if waitEvent, ok := result["wait_event"].(string); ok {
				query.WaitEvent = waitEvent
			}

			activeQueries = append(activeQueries, query)
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   activeQueries,
		})
	}
}
