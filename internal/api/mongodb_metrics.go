package api

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sefaphlvn/clustereye-test/internal/server"
)

// MongoDB System Metrics Handlers

// getMongoDBSystemCPUMetrics, MongoDB sistem CPU metriklerini getirir
func getMongoDBSystemCPUMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_system") |> filter(fn: (r) => r._field == "cpu_usage" or r._field == "cpu_cores") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_system") |> filter(fn: (r) => r._field == "cpu_usage" or r._field == "cpu_cores") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange)
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
				"error":  "MongoDB sistem CPU metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getMongoDBSystemMemoryMetrics, MongoDB sistem memory metriklerini getirir
func getMongoDBSystemMemoryMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_system") |> filter(fn: (r) => r._field == "memory_usage" or r._field == "total_memory" or r._field == "free_memory") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_system") |> filter(fn: (r) => r._field == "memory_usage" or r._field == "total_memory" or r._field == "free_memory") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange)
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
				"error":  "MongoDB sistem memory metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getMongoDBSystemDiskMetrics, MongoDB sistem disk metriklerini getirir
func getMongoDBSystemDiskMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_system") |> filter(fn: (r) => r._field == "total_disk" or r._field == "free_disk") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_system") |> filter(fn: (r) => r._field == "total_disk" or r._field == "free_disk") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange)
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
				"error":  "MongoDB sistem disk metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getMongoDBSystemResponseTimeMetrics, MongoDB sistem response time metriklerini getirir
func getMongoDBSystemResponseTimeMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_system") |> filter(fn: (r) => r._field == "response_time_ms") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 1m, fn: mean, createEmpty: false) |> sort(columns: ["_time"], desc: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_system") |> filter(fn: (r) => r._field == "response_time_ms") |> aggregateWindow(every: 1m, fn: mean, createEmpty: false) |> sort(columns: ["_time"], desc: false)`, timeRange)
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
				"error":  "MongoDB response time metrikleri alınamadı: " + err.Error(),
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

// MongoDB Database Metrics Handlers

// getMongoDBConnectionsMetrics, MongoDB bağlantı metriklerini getirir
func getMongoDBConnectionsMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_connections") |> filter(fn: (r) => r._field == "current" or r._field == "available" or r._field == "total_created_connections") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_connections") |> filter(fn: (r) => r._field == "current" or r._field == "available" or r._field == "total_created_connections") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange)
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
				"error":  "MongoDB bağlantı metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getMongoDBOperationsMetrics, MongoDB operasyon metriklerini getirir
func getMongoDBOperationsMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		database := c.Query("database") // İsteğe bağlı database filtresi
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		operationFields := "r._field == \"insert\" or r._field == \"query\" or r._field == \"update\" or r._field == \"delete\" or r._field == \"getmore\" or r._field == \"command\""

		if agentID != "" && database != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_operations") |> filter(fn: (r) => %s) |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> filter(fn: (r) => r.database_name =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, operationFields, regexp.QuoteMeta(agentID), regexp.QuoteMeta(database))
		} else if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_operations") |> filter(fn: (r) => %s) |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, operationFields, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_operations") |> filter(fn: (r) => %s) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, operationFields)
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
				"error":  "MongoDB operasyon metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getMongoDBOperationsRateMetrics, MongoDB operasyon rate metriklerini getirir (ops/sec)
func getMongoDBOperationsRateMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		database := c.Query("database") // İsteğe bağlı database filtresi
		timeRange := c.DefaultQuery("range", "1h")
		windowInterval := c.DefaultQuery("window", "1m") // Rate hesaplama penceresi

		var query string
		operationFields := "r._field == \"insert\" or r._field == \"query\" or r._field == \"update\" or r._field == \"delete\" or r._field == \"getmore\" or r._field == \"command\""

		if agentID != "" && database != "" {
			query = fmt.Sprintf(`
				from(bucket: "clustereye")
				|> range(start: -%s)
				|> filter(fn: (r) => r._measurement == "mongodb_operations")
				|> filter(fn: (r) => %s)
				|> filter(fn: (r) => r.agent_id =~ /^%s$/)
				|> filter(fn: (r) => r.database_name =~ /^%s$/)
				|> sort(columns: ["_time"])
				|> derivative(unit: 1s, nonNegative: true)
				|> aggregateWindow(every: %s, fn: mean, createEmpty: false)
			`, timeRange, operationFields, regexp.QuoteMeta(agentID), regexp.QuoteMeta(database), windowInterval)
		} else if agentID != "" {
			query = fmt.Sprintf(`
				from(bucket: "clustereye")
				|> range(start: -%s)
				|> filter(fn: (r) => r._measurement == "mongodb_operations")
				|> filter(fn: (r) => %s)
				|> filter(fn: (r) => r.agent_id =~ /^%s$/)
				|> sort(columns: ["_time"])
				|> derivative(unit: 1s, nonNegative: true)
				|> aggregateWindow(every: %s, fn: mean, createEmpty: false)
			`, timeRange, operationFields, regexp.QuoteMeta(agentID), windowInterval)
		} else {
			query = fmt.Sprintf(`
				from(bucket: "clustereye")
				|> range(start: -%s)
				|> filter(fn: (r) => r._measurement == "mongodb_operations")
				|> filter(fn: (r) => %s)
				|> sort(columns: ["_time"])
				|> derivative(unit: 1s, nonNegative: true)
				|> aggregateWindow(every: %s, fn: mean, createEmpty: false)
			`, timeRange, operationFields, windowInterval)
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
				"error":  "MongoDB operasyon rate metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		// Rate istatistikleri hesapla
		rateStats := calculateOperationRateStats(results, agentID)

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data": gin.H{
				"agent_id": agentID,
				"summary":  rateStats,
				"all_data": results,
				"note":     "Rates are calculated as operations per second using derivative function",
			},
		})
	}
}

// calculateOperationRateStats, operasyon rate istatistiklerini hesaplar
func calculateOperationRateStats(results []map[string]interface{}, agentID string) map[string]interface{} {
	stats := make(map[string]interface{})

	// Field'lara göre grupla
	fieldStats := make(map[string][]float64)
	var latestTime time.Time
	latestValues := make(map[string]float64)
	var latestTimestamp string

	for _, result := range results {
		if field, ok := result["_field"].(string); ok {
			if val, ok := result["_value"]; ok {
				if rate, ok := val.(float64); ok && rate >= 0 {
					fieldStats[field] = append(fieldStats[field], rate)

					// En son değerleri bul
					if timestamp, ok := result["_time"]; ok {
						if timeStr, ok := timestamp.(string); ok {
							if parsedTime, err := time.Parse(time.RFC3339, timeStr); err == nil {
								if parsedTime.After(latestTime) {
									latestTime = parsedTime
									latestValues[field] = rate
									latestTimestamp = timeStr
								}
							}
						}
					}
				}
			}
		}
	}

	// Her field için istatistik hesapla
	for field, values := range fieldStats {
		if len(values) > 0 {
			var sum, min, max float64
			min = values[0]
			max = values[0]

			for _, val := range values {
				sum += val
				if val < min {
					min = val
				}
				if val > max {
					max = val
				}
			}

			avg := sum / float64(len(values))

			stats[field+"_ops_per_sec"] = map[string]interface{}{
				"latest":      latestValues[field],
				"avg":         avg,
				"min":         min,
				"max":         max,
				"data_points": len(values),
			}
		}
	}

	// Toplam operasyon rate'i hesapla
	var totalLatestRate float64
	for _, rate := range latestValues {
		totalLatestRate += rate
	}

	stats["total_ops_per_sec"] = totalLatestRate
	stats["latest_timestamp"] = latestTimestamp
	stats["operations_tracked"] = len(fieldStats)

	return stats
}

// getMongoDBStorageMetrics, MongoDB storage metriklerini getirir
func getMongoDBStorageMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		database := c.Query("database") // İsteğe bağlı database filtresi
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		storageFields := "r._field == \"data_size_mb\" or r._field == \"storage_size_mb\" or r._field == \"index_size_mb\" or r._field == \"avg_obj_size\" or r._field == \"file_size_mb\""

		if agentID != "" && database != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_storage") |> filter(fn: (r) => %s) |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> filter(fn: (r) => r.database_name =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, storageFields, regexp.QuoteMeta(agentID), regexp.QuoteMeta(database))
		} else if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_storage") |> filter(fn: (r) => %s) |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, storageFields, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_storage") |> filter(fn: (r) => %s) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, storageFields)
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
				"error":  "MongoDB storage metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getMongoDBDatabaseInfoMetrics, MongoDB database info metriklerini getirir
func getMongoDBDatabaseInfoMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		database := c.Query("database") // İsteğe bağlı database filtresi
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if agentID != "" && database != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_database") |> filter(fn: (r) => r._field == "database_count" or r._field == "collection_count") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> filter(fn: (r) => r.database_name =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID), regexp.QuoteMeta(database))
		} else if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_database") |> filter(fn: (r) => r._field == "database_count" or r._field == "collection_count") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_database") |> filter(fn: (r) => r._field == "database_count" or r._field == "collection_count") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange)
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
				"error":  "MongoDB database info metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// MongoDB Replication Metrics Handlers

// getMongoDBReplicationStatusMetrics, MongoDB replication durum metriklerini getirir
func getMongoDBReplicationStatusMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		replicaSetName := c.Query("replica_set") // İsteğe bağlı replica set filtresi
		nodeRole := c.Query("node_role")         // İsteğe bağlı node role filtresi
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if agentID != "" && replicaSetName != "" && nodeRole != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_replication") |> filter(fn: (r) => r._field == "member_state" or r._field == "members_healthy" or r._field == "members_total") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> filter(fn: (r) => r.replica_set_name =~ /^%s$/) |> filter(fn: (r) => r.node_role =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID), regexp.QuoteMeta(replicaSetName), regexp.QuoteMeta(nodeRole))
		} else if agentID != "" && replicaSetName != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_replication") |> filter(fn: (r) => r._field == "member_state" or r._field == "members_healthy" or r._field == "members_total") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> filter(fn: (r) => r.replica_set_name =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID), regexp.QuoteMeta(replicaSetName))
		} else if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_replication") |> filter(fn: (r) => r._field == "member_state" or r._field == "members_healthy" or r._field == "members_total") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_replication") |> filter(fn: (r) => r._field == "member_state" or r._field == "members_healthy" or r._field == "members_total") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange)
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
				"error":  "MongoDB replication durum metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getMongoDBReplicationLagMetrics, MongoDB replication lag metriklerini getirir
func getMongoDBReplicationLagMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		replicaSetName := c.Query("replica_set") // İsteğe bağlı replica set filtresi
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if agentID != "" && replicaSetName != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_replication") |> filter(fn: (r) => r._field == "lag_ms_num") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> filter(fn: (r) => r.replica_set_name =~ /^%s$/) |> aggregateWindow(every: 1m, fn: mean, createEmpty: false) |> sort(columns: ["_time"], desc: false)`, timeRange, regexp.QuoteMeta(agentID), regexp.QuoteMeta(replicaSetName))
		} else if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_replication") |> filter(fn: (r) => r._field == "lag_ms_num") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 1m, fn: mean, createEmpty: false) |> sort(columns: ["_time"], desc: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_replication") |> filter(fn: (r) => r._field == "lag_ms_num") |> aggregateWindow(every: 1m, fn: mean, createEmpty: false) |> sort(columns: ["_time"], desc: false)`, timeRange)
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
				"error":  "MongoDB replication lag metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		// Replication lag istatistikleri hesapla
		var totalLag, minLag, maxLag, avgLag float64
		var lagCount int
		var latestTime time.Time
		var latestLag float64
		var latestTimestamp string

		if len(results) > 0 {
			minLag = 99999999 // Büyük bir başlangıç değeri
			maxLag = 0

			for _, result := range results {
				if val, ok := result["_value"]; ok {
					if lagMs, ok := val.(float64); ok && lagMs >= 0 {
						// Milisaniyeden saniyeye çevir
						lagSeconds := lagMs / 1000.0

						totalLag += lagSeconds
						lagCount++

						if lagSeconds < minLag {
							minLag = lagSeconds
						}
						if lagSeconds > maxLag {
							maxLag = lagSeconds
						}

						// En son zaman damgasını bul
						if timestamp, ok := result["_time"]; ok {
							if timeStr, ok := timestamp.(string); ok {
								if parsedTime, err := time.Parse(time.RFC3339, timeStr); err == nil {
									if parsedTime.After(latestTime) {
										latestTime = parsedTime
										latestLag = lagSeconds
										latestTimestamp = timeStr
									}
								}
							}
						}
					}
				}
			}

			if lagCount > 0 {
				avgLag = totalLag / float64(lagCount)
			}

			// Eğer hiç veri yoksa minimum değeri sıfırlayalım
			if lagCount == 0 {
				minLag = 0
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data": gin.H{
				"agent_id": agentID,
				"summary": gin.H{
					"latest_replication_lag_seconds": latestLag,
					"latest_timestamp":               latestTimestamp,
					"avg_replication_lag_seconds":    avgLag,
					"min_replication_lag_seconds":    minLag,
					"max_replication_lag_seconds":    maxLag,
					"total_measurements":             lagCount,
				},
				"all_data": results,
			},
		})
	}
}

// getMongoDBOplogMetrics, MongoDB oplog metriklerini getirir
func getMongoDBOplogMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		replicaSetName := c.Query("replica_set") // İsteğe bağlı replica set filtresi
		timeRange := c.DefaultQuery("range", "1h")

		// Oplog ile ilgili tüm field'ları dahil et
		oplogFields := "r._field == \"oplog_size_mb\" or r._field == \"oplog_count\" or r._field == \"oplog_max_size_mb\" or r._field == \"oplog_storage_mb\" or r._field == \"oplog_utilization_percent\""

		var query string
		if agentID != "" && replicaSetName != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_replication") |> filter(fn: (r) => %s) |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> filter(fn: (r) => r.replica_set_name =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, oplogFields, regexp.QuoteMeta(agentID), regexp.QuoteMeta(replicaSetName))
		} else if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_replication") |> filter(fn: (r) => %s) |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, oplogFields, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_replication") |> filter(fn: (r) => %s) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, oplogFields)
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
				"error":  "MongoDB oplog metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// MongoDB Performance Metrics Handlers

// getMongoDBPerformanceQPSMetrics, MongoDB QPS (queries per second) metriklerini getirir
func getMongoDBPerformanceQPSMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		database := c.Query("database") // İsteğe bağlı database filtresi
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if agentID != "" && database != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_performance") |> filter(fn: (r) => r._field == "queries_per_sec") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> filter(fn: (r) => r.database_name =~ /^%s$/) |> aggregateWindow(every: 1m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID), regexp.QuoteMeta(database))
		} else if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_performance") |> filter(fn: (r) => r._field == "queries_per_sec") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 1m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_performance") |> filter(fn: (r) => r._field == "queries_per_sec") |> aggregateWindow(every: 1m, fn: mean, createEmpty: false)`, timeRange)
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
				"error":  "MongoDB QPS metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getMongoDBPerformanceReadWriteRatioMetrics, MongoDB Read/Write oranı metriklerini getirir
func getMongoDBPerformanceReadWriteRatioMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		database := c.Query("database")
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if agentID != "" && database != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_performance") |> filter(fn: (r) => r._field == "read_write_ratio") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> filter(fn: (r) => r.database_name =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID), regexp.QuoteMeta(database))
		} else if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_performance") |> filter(fn: (r) => r._field == "read_write_ratio") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_performance") |> filter(fn: (r) => r._field == "read_write_ratio") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange)
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
				"error":  "MongoDB Read/Write oranı metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getMongoDBPerformanceSlowQueriesMetrics, MongoDB yavaş query metriklerini getirir
func getMongoDBPerformanceSlowQueriesMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		database := c.Query("database")
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if agentID != "" && database != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_performance") |> filter(fn: (r) => r._field == "slow_queries_count") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> filter(fn: (r) => r.database_name =~ /^%s$/) |> aggregateWindow(every: 5m, fn: sum, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID), regexp.QuoteMeta(database))
		} else if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_performance") |> filter(fn: (r) => r._field == "slow_queries_count") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: sum, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_performance") |> filter(fn: (r) => r._field == "slow_queries_count") |> aggregateWindow(every: 5m, fn: sum, createEmpty: false)`, timeRange)
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
				"error":  "MongoDB yavaş query metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getMongoDBPerformanceQueryTimeMetrics, MongoDB query süresi metriklerini getirir (avg, p95, p99)
func getMongoDBPerformanceQueryTimeMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		database := c.Query("database")
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		queryTimeFields := "r._field == \"avg_query_time_ms\" or r._field == \"query_time_p95_ms\" or r._field == \"query_time_p99_ms\""

		if agentID != "" && database != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_performance") |> filter(fn: (r) => %s) |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> filter(fn: (r) => r.database_name =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, queryTimeFields, regexp.QuoteMeta(agentID), regexp.QuoteMeta(database))
		} else if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_performance") |> filter(fn: (r) => %s) |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, queryTimeFields, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_performance") |> filter(fn: (r) => %s) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, queryTimeFields)
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
				"error":  "MongoDB query süresi metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getMongoDBPerformanceActiveQueriesMetrics, MongoDB aktif query metriklerini getirir
func getMongoDBPerformanceActiveQueriesMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		database := c.Query("database")
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if agentID != "" && database != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_performance") |> filter(fn: (r) => r._field == "active_queries_count") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> filter(fn: (r) => r.database_name =~ /^%s$/) |> aggregateWindow(every: 1m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID), regexp.QuoteMeta(database))
		} else if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_performance") |> filter(fn: (r) => r._field == "active_queries_count") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 1m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_performance") |> filter(fn: (r) => r._field == "active_queries_count") |> aggregateWindow(every: 1m, fn: mean, createEmpty: false)`, timeRange)
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
				"error":  "MongoDB aktif query metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getMongoDBPerformanceProfilerMetrics, MongoDB profiler metriklerini getirir
func getMongoDBPerformanceProfilerMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_performance") |> filter(fn: (r) => r._field == "profiler_enabled_dbs") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_performance") |> filter(fn: (r) => r._field == "profiler_enabled_dbs") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange)
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
				"error":  "MongoDB profiler metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getMongoDBPerformanceAllMetrics, tüm MongoDB performance metriklerini tek seferde getirir
func getMongoDBPerformanceAllMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		database := c.Query("database")
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if agentID != "" && database != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_performance") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> filter(fn: (r) => r.database_name =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID), regexp.QuoteMeta(database))
		} else if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_performance") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mongodb_performance") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange)
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
				"error":  "MongoDB performance metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}
