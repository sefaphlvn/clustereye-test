package api

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sefaphlvn/clustereye-test/internal/server"
)

// PostgreSQL System Metrics Handlers

// getPostgreSQLSystemCPUMetrics, PostgreSQL sistem CPU metriklerini getirir
func getPostgreSQLSystemCPUMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_system") |> filter(fn: (r) => r._field == "cpu_usage" or r._field == "cpu_cores") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
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

		var query string
		if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_system") |> filter(fn: (r) => r._field == "free_memory" or r._field == "total_memory") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
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

		var query string
		if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_system") |> filter(fn: (r) => r._field == "free_disk" or r._field == "total_disk") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
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

		var query string
		if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_system") |> filter(fn: (r) => r._field == "response_time_ms") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 1m, fn: mean, createEmpty: false) |> sort(columns: ["_time"], desc: false)`, timeRange, regexp.QuoteMeta(agentID))
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

		var query string
		if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_connections") |> filter(fn: (r) => r._field == "active" or r._field == "idle" or r._field == "idle_in_transaction" or r._field == "total") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_connections") |> filter(fn: (r) => r._field == "active" or r._field == "idle" or r._field == "idle_in_transaction" or r._field == "total") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange)
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

		var query string
		if agentID != "" && database != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_database") |> filter(fn: (r) => r._field == "xact_commit" or r._field == "xact_rollback") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> filter(fn: (r) => r.database =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID), regexp.QuoteMeta(database))
		} else if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_database") |> filter(fn: (r) => r._field == "xact_commit" or r._field == "xact_rollback") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
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

		var query string
		if agentID != "" && database != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_database") |> filter(fn: (r) => r._field == "blks_hit" or r._field == "blks_read") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> filter(fn: (r) => r.database =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID), regexp.QuoteMeta(database))
		} else if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_database") |> filter(fn: (r) => r._field == "blks_hit" or r._field == "blks_read") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
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
					if value, ok := result["_value"].(float64); ok {
						if field == "blks_hit" {
							blksHitData[timestamp] = value
						} else if field == "blks_read" {
							blksReadData[timestamp] = value
						}
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

		var query string
		if agentID != "" && database != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_database") |> filter(fn: (r) => r._field == "deadlocks") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> filter(fn: (r) => r.database =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID), regexp.QuoteMeta(database))
		} else if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_database") |> filter(fn: (r) => r._field == "deadlocks") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
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

		// Replication lag için özel measurement olabileceğini varsayıyoruz
		// Eğer mevcut değilse postgresql_system'de replication_lag field'ı olabilir
		var query string
		if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_replication" or (r._measurement == "postgresql_system" and r._field == "replication_lag")) |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_replication" or (r._measurement == "postgresql_system" and r._field == "replication_lag")) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange)
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

		var query string
		if agentID != "" && lockType != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_locks") |> filter(fn: (r) => r._field == "granted" or r._field == "waiting") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> filter(fn: (r) => r.lock_type =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID), regexp.QuoteMeta(lockType))
		} else if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_locks") |> filter(fn: (r) => r._field == "granted" or r._field == "waiting") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
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

		// Limit'i integer'a çevir
		limitInt, err := strconv.Atoi(limit)
		if err != nil || limitInt <= 0 {
			limitInt = 100
		}

		// PostgreSQL table metrikleri için özel measurement
		var query string
		if agentID != "" && database != "" && tableName != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_tables") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> filter(fn: (r) => r.database =~ /^%s$/) |> filter(fn: (r) => r.table =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false) |> limit(n: %d)`, timeRange, regexp.QuoteMeta(agentID), regexp.QuoteMeta(database), regexp.QuoteMeta(tableName), limitInt)
		} else if agentID != "" && database != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_tables") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> filter(fn: (r) => r.database =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false) |> limit(n: %d)`, timeRange, regexp.QuoteMeta(agentID), regexp.QuoteMeta(database), limitInt)
		} else if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_tables") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false) |> limit(n: %d)`, timeRange, regexp.QuoteMeta(agentID), limitInt)
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

		// Limit'i integer'a çevir
		limitInt, err := strconv.Atoi(limit)
		if err != nil || limitInt <= 0 {
			limitInt = 100
		}

		// PostgreSQL index metrikleri için özel measurement
		var query string
		if agentID != "" && database != "" && indexName != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_indexes") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> filter(fn: (r) => r.database =~ /^%s$/) |> filter(fn: (r) => r.index =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false) |> limit(n: %d)`, timeRange, regexp.QuoteMeta(agentID), regexp.QuoteMeta(database), regexp.QuoteMeta(indexName), limitInt)
		} else if agentID != "" && database != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_indexes") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> filter(fn: (r) => r.database =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false) |> limit(n: %d)`, timeRange, regexp.QuoteMeta(agentID), regexp.QuoteMeta(database), limitInt)
		} else if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_indexes") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false) |> limit(n: %d)`, timeRange, regexp.QuoteMeta(agentID), limitInt)
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "postgresql_indexes") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false) |> limit(n: %d)`, timeRange, limitInt)
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
