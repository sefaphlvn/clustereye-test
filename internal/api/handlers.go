package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sefaphlvn/clustereye-test/internal/server"
	pb "github.com/sefaphlvn/clustereye-test/pkg/agent"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// RegisterHandlers, API rotalarını Gin router'a kaydeder
func RegisterHandlers(router *gin.Engine, server *server.Server) {
	// API grupları oluştur
	v1 := router.Group("/api/v1")

	// Login endpoint'i
	v1.POST("/login", Login(server.GetDB()))

	// Kullanıcı işlemleri endpoint'leri
	v1.POST("/users", CreateUser(server.GetDB()))
	// Kullanıcı listesini getir - Sadece admin erişebilir
	v1.GET("/users", GetUsers(server.GetDB()))
	// Belirli bir kullanıcıyı getir
	v1.GET("/users/:id", GetUser(server.GetDB()))
	// Kullanıcıyı güncelle
	v1.PUT("/users/:id", UpdateUser(server.GetDB()))
	// Kullanıcıyı sil - Sadece admin erişebilir
	v1.DELETE("/users/:id", AuthMiddleware(), DeleteUser(server.GetDB()))

	// 2FA Endpoint'leri
	twofa := v1.Group("/2fa")
	{
		// 2FA durumunu getir
		twofa.GET("/status", AuthMiddleware(), Get2FAStatus(server.GetDB()))
		// 2FA secret oluştur
		twofa.POST("/generate", AuthMiddleware(), Generate2FASecret(server.GetDB()))
		// 2FA'yı aktif et
		twofa.POST("/enable", AuthMiddleware(), Enable2FA(server.GetDB()))
		// 2FA'yı deaktif et
		twofa.POST("/disable", AuthMiddleware(), Disable2FA(server.GetDB()))
		// Backup kodları yeniden oluştur
		twofa.POST("/regenerate-backup-codes", AuthMiddleware(), RegenerateBackupCodes(server.GetDB()))
	}

	// Agent Endpoint'leri
	agents := v1.Group("/agents")
	{
		// Tüm bağlı agent'ları listele
		agents.GET("", getAgents(server))

		// Agent versiyon bilgilerini getir
		agents.GET("/versions", getAgentVersions(server))

		// Agent'a sorgu gönder
		agents.POST("/:agent_id/query", sendQueryToAgent(server))

		// Agent'dan sistem metriklerini al
		agents.POST("/:agent_id/metrics", sendMetricsRequestToAgent(server))

		// MongoDB log dosyalarını listele
		agents.GET("/:agent_id/mongo/logs", listMongoLogs(server))

		// PostgreSQL log dosyalarını listele
		agents.GET("/:agent_id/postgres/logs", listPostgresLogs(server))

		// MongoDB log dosyasını analiz et
		agents.POST("/:agent_id/mongo/logs/analyze", analyzeMongoLog(server))

		// PostgreSQL log dosyasını analiz et
		agents.POST("/:agent_id/postgres/logs/analyze", analyzePostgresLog(server))

		// PostgreSQL config dosyasını okumasını ister
		agents.POST("/:agent_id/postgres/config", readPostgresConfig(server))

		// Explain Query endpoint'i - PostgreSQL sorgu planını analiz et
		agents.POST("/:agent_id/explain", explainQuery(server))

		// MongoDB Explain Query endpoint'i - MongoDB sorgu planını analiz et
		agents.POST("/:agent_id/mongo/explain", explainMongoQuery(server))

		// MSSQL Explain Query endpoint'i - MSSQL sorgu planını XML formatında döndürür
		agents.POST("/:agent_id/mssql/explain", explainMssqlQuery(server))

		// MSSQL Best Practices analiz endpoint'i
		agents.GET("/:agent_id/mssql/bestpractices", getMSSQLBestPracticesAnalysis(server))
	}

	// Status Endpoint'leri
	status := v1.Group("/status")
	{
		// PostgreSQL durum bilgilerini getir
		status.GET("/postgres", getPostgresStatus(server))
		// MongoDB durum bilgilerini getir
		status.GET("/mongo", getMongoStatus(server))
		// MSSQL durum bilgilerini getir
		status.GET("/mssql", getMSSQLStatus(server))
		// Agent durum bilgilerini getir
		status.GET("/agents", getAgentStatus(server))
		// Tüm node sağlık bilgilerini getir
		status.GET("/nodeshealth", getNodesHealth(server))
		// Alarm listesini getir
		status.GET("/alarms", getAlarms(server))
		// Dashboard için optimize edilmiş recent alarms
		status.GET("/alarms/recent", getRecentAlarms(server))
		// Alarm endpoint'leri
		status.POST("/alarms/:event_id/acknowledge", acknowledgeAlarm(server))
	}

	// Notification Settings Endpoint'leri
	notification := v1.Group("/notification-settings")
	{
		// Notification ayarlarını getir - Sadece admin erişebilir
		notification.GET("", AuthMiddleware(), GetNotificationSettings(server.GetDB()))
		// Notification ayarlarını güncelle - Sadece admin erişebilir
		notification.POST("", AuthMiddleware(), UpdateNotificationSettings(server.GetDB()))
		// Slack webhook'unu test et - Sadece admin erişebilir
		notification.POST("/test-slack", AuthMiddleware(), TestSlackNotification(server.GetDB()))
	}

	// Threshold Settings Endpoint'leri
	threshold := v1.Group("/threshold-settings")
	{
		// Threshold ayarlarını getir - Sadece admin erişebilir
		threshold.GET("", AuthMiddleware(), GetThresholdSettings(server.GetDB()))
		// Threshold ayarlarını güncelle - Sadece admin erişebilir
		threshold.POST("", AuthMiddleware(), UpdateThresholdSettings(server.GetDB()))
	}

	// Lisans Bilgileri Endpoint'leri
	licenses := v1.Group("/licenses")
	{
		// Lisans bilgilerini getir - Sadece admin erişebilir
		licenses.GET("", AuthMiddleware(), GetLicences(server.GetDB()))
	}

	// Job Endpoint'leri
	jobs := v1.Group("/jobs")
	{
		// Genel job oluşturma endpoint'i
		jobs.POST("", createJob(server))
		// MongoDB primary promotion
		jobs.POST("/mongo/promote-primary", promoteMongoToPrimary(server))
		// MongoDB secondary freeze
		jobs.POST("/mongo/freeze-secondary", freezeMongoSecondary(server))
		// PostgreSQL master promotion
		jobs.POST("/postgres/promote-master", promotePostgresToMaster(server))
		// PostgreSQL convert to slave
		jobs.POST("/postgres/convert-to-slave", convertPostgresToSlave(server))
		// Job durumunu sorgula
		jobs.GET("/:job_id", getJob(server))
		// Job listesini getir
		jobs.GET("", listJobs(server))
	}

	// Process Logs Endpoint'leri
	processLogs := v1.Group("/process-logs")
	{
		// İşlem loglarını getir
		processLogs.GET("", getProcessLogs(server))
	}

	// Metrics Endpoint'leri
	metrics := v1.Group("/metrics")
	{
		// Metrik sorgulama endpoint'i
		metrics.POST("/query", queryMetrics(server))
		// CPU metrikleri
		metrics.GET("/cpu", getCPUMetrics(server))
		// Memory metrikleri
		metrics.GET("/memory", getMemoryMetrics(server))
		// Disk metrikleri
		metrics.GET("/disk", getDiskMetrics(server))
		// Network metrikleri
		metrics.GET("/network", getNetworkMetrics(server))
		// Database metrikleri
		metrics.GET("/database", getDatabaseMetrics(server))
		// Genel dashboard metrikleri
		metrics.GET("/dashboard", getDashboardMetrics(server))

		// PostgreSQL özel metrikleri
		postgresql := metrics.Group("/postgresql")
		{
			// PostgreSQL sistem metrikleri
			system := postgresql.Group("/system")
			{
				system.GET("/cpu", getPostgreSQLSystemCPUMetrics(server))
				system.GET("/memory", getPostgreSQLSystemMemoryMetrics(server))
				system.GET("/disk", getPostgreSQLSystemDiskMetrics(server))
				system.GET("/response-time", getPostgreSQLSystemResponseTimeMetrics(server))
			}

			// PostgreSQL veritabanı metrikleri
			database := postgresql.Group("/database")
			{
				database.GET("/connections", getPostgreSQLConnectionsMetrics(server))
				database.GET("/transactions", getPostgreSQLTransactionsMetrics(server))
				database.GET("/cache", getPostgreSQLCacheMetrics(server))
				database.GET("/deadlocks", getPostgreSQLDeadlocksMetrics(server))
				database.GET("/replication", getPostgreSQLReplicationMetrics(server))
				database.GET("/locks", getPostgreSQLLocksMetrics(server))
				database.GET("/tables", getPostgreSQLTablesMetrics(server))
				database.GET("/indexes", getPostgreSQLIndexesMetrics(server))
			}

			// PostgreSQL performance metrikleri
			performance := postgresql.Group("/performance")
			{
				performance.GET("/qps", getPostgreSQLPerformanceQPSMetrics(server))
				performance.GET("/query-time", getPostgreSQLPerformanceQueryTimeMetrics(server))
				performance.GET("/slow-queries", getPostgreSQLPerformanceSlowQueriesMetrics(server))
				performance.GET("/active-queries", getPostgreSQLPerformanceActiveQueriesMetrics(server))
				performance.GET("/long-running-queries", getPostgreSQLPerformanceLongRunningQueriesMetrics(server))
				performance.GET("/read-write-ratio", getPostgreSQLPerformanceReadWriteRatioMetrics(server))
				performance.GET("/index-scan-ratio", getPostgreSQLPerformanceIndexScanRatioMetrics(server))
				performance.GET("/seq-scan-ratio", getPostgreSQLPerformanceSeqScanRatioMetrics(server))
				performance.GET("/cache-hit-ratio", getPostgreSQLPerformanceCacheHitRatioMetrics(server))
				performance.GET("/all", getPostgreSQLPerformanceAllMetrics(server))
			}
		}

		// MongoDB özel metrikleri
		mongodb := metrics.Group("/mongodb")
		{
			// MongoDB dashboard - tüm metrikleri tek seferde
			mongodb.GET("/dashboard", getMongoDBDashboardMetrics(server))

			// MongoDB sistem metrikleri
			system := mongodb.Group("/system")
			{
				system.GET("/cpu", getMongoDBSystemCPUMetrics(server))
				system.GET("/memory", getMongoDBSystemMemoryMetrics(server))
				system.GET("/disk", getMongoDBSystemDiskMetrics(server))
				system.GET("/response-time", getMongoDBSystemResponseTimeMetrics(server))
			}

			// MongoDB veritabanı metrikleri
			database := mongodb.Group("/database")
			{
				database.GET("/connections", getMongoDBConnectionsMetrics(server))
				database.GET("/operations", getMongoDBOperationsMetrics(server))
				database.GET("/operations-rate", getMongoDBOperationsRateMetrics(server))
				database.GET("/storage", getMongoDBStorageMetrics(server))
				database.GET("/info", getMongoDBDatabaseInfoMetrics(server))
			}

			// MongoDB replikasyon metrikleri
			replication := mongodb.Group("/replication")
			{
				replication.GET("/status", getMongoDBReplicationStatusMetrics(server))
				replication.GET("/lag", getMongoDBReplicationLagMetrics(server))
				replication.GET("/oplog", getMongoDBOplogMetrics(server))
			}

			// MongoDB performance metrikleri
			performance := mongodb.Group("/performance")
			{
				performance.GET("/qps", getMongoDBPerformanceQPSMetrics(server))
				performance.GET("/read-write-ratio", getMongoDBPerformanceReadWriteRatioMetrics(server))
				performance.GET("/slow-queries", getMongoDBPerformanceSlowQueriesMetrics(server))
				performance.GET("/query-time", getMongoDBPerformanceQueryTimeMetrics(server))
				performance.GET("/active-queries", getMongoDBPerformanceActiveQueriesMetrics(server))
				performance.GET("/profiler", getMongoDBPerformanceProfilerMetrics(server))
				performance.GET("/all", getMongoDBPerformanceAllMetrics(server))
			}
		}
	}

	// MSSQL özel endpoint'leri
	mssql := v1.Group("/mssql")
	{
		mssql.GET("/cpu", getMSSQLCPUMetrics(server))
		mssql.GET("/connections", getMSSQLConnectionsMetrics(server))
		mssql.GET("/system", getMSSQLSystemMetrics(server))
		mssql.GET("/database", getMSSQLDatabaseMetrics(server))
		mssql.GET("/blocking", getMSSQLBlockingMetrics(server))
		mssql.GET("/wait", getMSSQLWaitMetrics(server))
		mssql.GET("/deadlock", getMSSQLDeadlockMetrics(server))
		mssql.GET("/response-time", getMSSQLResponseTimeMetrics(server))
		mssql.GET("/performance", getMSSQLPerformanceMetrics(server))
		mssql.GET("/performance-rates", getMSSQLPerformanceRateMetrics(server))
		mssql.GET("/transactions", getMSSQLTransactionsMetrics(server))
		mssql.GET("/transactions-rates", getMSSQLTransactionsRateMetrics(server))
		mssql.GET("/all", getMSSQLAllMetrics(server))
	}

	// Debug Endpoint'leri
	debug := v1.Group("/debug")
	{
		// InfluxDB'de mevcut field'ları listeler (debug amaçlı)
		debug.GET("/fields", getAvailableFields(server))
		// InfluxDB'de mevcut measurement'ları listeler (debug amaçlı)
		debug.GET("/measurements", getAvailableMeasurements(server))
		// Coordination state management (admin/debug)
		debug.GET("/coordination/status", getCoordinationStatus(server))
		debug.POST("/coordination/cleanup", cleanupCoordinationState(server))
	}
}

// getAgents, bağlı tüm agent'ları listeler
func getAgents(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agents := server.GetConnectedAgents()

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data": gin.H{
				"agents": agents,
			},
		})
	}
}

// sendQueryToAgent, belirli bir agent'a sorgu gönderir
func sendQueryToAgent(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Param("agent_id")

		if agentID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "agent_id parametresi gerekli",
			})
			return
		}

		var req struct {
			QueryID  string `json:"query_id" binding:"required"`
			Command  string `json:"command" binding:"required"`
			Database string `json:"database"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Geçersiz JSON verisi: " + err.Error(),
			})
			return
		}

		// Context oluştur (request'in iptal edilmesi durumunda kullanılacak)
		ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
		defer cancel()

		// Sorguyu gönder ve cevabı bekle
		result, err := server.SendQuery(ctx, agentID, req.QueryID, req.Command, req.Database)
		if err != nil {
			status := http.StatusInternalServerError
			message := "Sorgu sırasında bir hata oluştu: " + err.Error()

			if err == context.DeadlineExceeded {
				status = http.StatusGatewayTimeout
				message = "Sorgu zaman aşımına uğradı"
			} else if err.Error() == http.ErrNoLocation.Error() {
				status = http.StatusNotFound
				message = "Agent bulunamadı veya bağlantı kapalı"
			}

			c.JSON(status, gin.H{
				"status": "error",
				"error":  message,
			})
			return
		}

		// Başarılı yanıt
		c.JSON(http.StatusOK, gin.H{
			"status":   "success",
			"agent_id": agentID,
			"query_id": result.QueryId,
			"result":   result.Result,
		})
	}
}

// getPostgresStatus, PostgreSQL durum bilgilerini getirir
func getPostgresStatus(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		result, err := server.GetStatusPostgres(ctx, nil)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "PostgreSQL durum bilgileri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   result.AsInterface(),
		})
	}
}

// getMongoStatus, MongoDB durum bilgilerini getirir
func getMongoStatus(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		result, err := server.GetStatusMongo(ctx, nil)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "MongoDB durum bilgileri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   result.AsInterface(),
		})
	}
}

// getAgentStatus, bağlı agent'ların durumunu getirir
func getAgentStatus(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		// gRPC bağlantılarından agent durumlarını al
		agents := server.GetConnectedAgents()

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data": gin.H{
				"agents": agents,
			},
		})
	}
}

// sendMetricsRequestToAgent, agent'a sistem metrikleri isteği gönderir
func sendMetricsRequestToAgent(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Param("agent_id")
		if agentID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "agent_id parametresi gerekli",
			})
			return
		}

		log.Printf("[INFO] Metrik isteği başlatılıyor - Agent ID: %s", agentID)

		// Sadece 5 saniyelik kısa bir timeout kullan
		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()
		// Metrikleri al
		req := &pb.SystemMetricsRequest{
			AgentId: agentID,
		}

		// Server'a isteği gönder ve yanıtı bekle
		response, err := server.SendSystemMetrics(ctx, req)
		if err != nil {
			log.Printf("[ERROR] Metrik alma işlemi başarısız: %v", err)

			// Context timeout ise 504 dön
			if ctx.Err() == context.DeadlineExceeded {
				c.JSON(http.StatusGatewayTimeout, gin.H{
					"status": "error",
					"error":  "Metrik toplama zaman aşımına uğradı",
				})
				return
			}

			// Diğer hatalar için 500 dön
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  fmt.Sprintf("Metrik toplama hatası: %v", err),
			})
			return
		}

		// Başarılı yanıt
		log.Printf("[INFO] Metrikler başarıyla alındı - Agent ID: %s", agentID)
		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   response.Data,
		})
	}
}

// listMongoLogs, belirtilen agent'tan MongoDB log dosyalarını listeler
func listMongoLogs(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Param("agent_id")

		if agentID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "agent_id parametresi gerekli",
			})
			return
		}

		// Context oluştur ve agent_id ekle
		ctx := context.WithValue(c.Request.Context(), "agent_id", agentID)
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		// MongoDB log dosyalarını listele
		req := &pb.MongoLogListRequest{}

		response, err := server.ListMongoLogs(ctx, req)
		if err != nil {
			httpStatus := http.StatusInternalServerError
			message := "MongoDB log dosyaları listelenirken bir hata oluştu: " + err.Error()

			if err == context.DeadlineExceeded {
				httpStatus = http.StatusGatewayTimeout
				message = "İstek zaman aşımına uğradı"
			} else if st, ok := grpcstatus.FromError(err); ok {
				if st.Code() == codes.NotFound {
					c.JSON(http.StatusNotFound, gin.H{
						"status": "error",
						"error":  st.Message(),
					})
					return
				}
				message = st.Message()
			}

			c.JSON(httpStatus, gin.H{
				"status": "error",
				"error":  message,
			})
			return
		}

		// Dosya bilgilerini daha okunabilir hale getir
		logFiles := make([]map[string]interface{}, 0, len(response.LogFiles))
		for _, file := range response.LogFiles {
			logFiles = append(logFiles, map[string]interface{}{
				"name":                   file.Name,
				"path":                   file.Path,
				"size":                   file.Size,
				"size_readable":          formatBytes(file.Size),
				"last_modified":          file.LastModified,
				"last_modified_readable": time.Unix(file.LastModified, 0).Format(time.RFC3339),
			})
		}

		// Başarılı yanıt
		c.JSON(http.StatusOK, gin.H{
			"status":    "success",
			"agent_id":  agentID,
			"log_files": logFiles,
		})
	}
}

// listPostgresLogs, belirtilen agent'tan PostgreSQL log dosyalarını listeler
func listPostgresLogs(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Param("agent_id")

		if agentID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "agent_id parametresi gerekli",
			})
			return
		}

		// Context oluştur ve agent_id ekle
		ctx := context.WithValue(c.Request.Context(), "agent_id", agentID)
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		// PostgreSQL log dosyalarını listele
		req := &pb.PostgresLogListRequest{
			AgentId: agentID,
			LogPath: c.Query("log_path"), // Optional query parameter for log path
		}

		response, err := server.ListPostgresLogs(ctx, req)
		if err != nil {
			httpStatus := http.StatusInternalServerError
			message := "PostgreSQL log dosyaları listelenirken bir hata oluştu: " + err.Error()

			if err == context.DeadlineExceeded {
				httpStatus = http.StatusGatewayTimeout
				message = "İstek zaman aşımına uğradı"
			} else if st, ok := grpcstatus.FromError(err); ok {
				if st.Code() == codes.NotFound {
					c.JSON(http.StatusNotFound, gin.H{
						"status": "error",
						"error":  st.Message(),
					})
					return
				}
				message = st.Message()
			}

			c.JSON(httpStatus, gin.H{
				"status": "error",
				"error":  message,
			})
			return
		}

		// Dosya bilgilerini daha okunabilir hale getir
		logFiles := make([]map[string]interface{}, 0, len(response.LogFiles))
		for _, file := range response.LogFiles {
			logFiles = append(logFiles, map[string]interface{}{
				"name":                   file.Name,
				"path":                   file.Path,
				"size":                   file.Size,
				"size_readable":          formatBytes(file.Size),
				"last_modified":          file.LastModified,
				"last_modified_readable": time.Unix(file.LastModified, 0).Format(time.RFC3339),
			})
		}

		// Başarılı yanıt
		c.JSON(http.StatusOK, gin.H{
			"status":    "success",
			"agent_id":  agentID,
			"log_files": logFiles,
		})
	}
}

// analyzeMongoLog, belirtilen agent'tan MongoDB log dosyasını analiz etmesini ister
func analyzeMongoLog(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Param("agent_id")

		if agentID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "agent_id parametresi gerekli",
			})
			return
		}

		var req struct {
			LogFilePath        string `json:"log_file_path" binding:"required"`
			SlowQueryThreshold int64  `json:"slow_query_threshold_ms"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Geçersiz JSON verisi: " + err.Error(),
			})
			return
		}

		// Context oluştur ve agent_id ekle
		ctx := context.WithValue(c.Request.Context(), "agent_id", agentID)
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()

		// MongoDB log dosyasını analiz et
		analyzeReq := &pb.MongoLogAnalyzeRequest{
			LogFilePath:          req.LogFilePath,
			SlowQueryThresholdMs: req.SlowQueryThreshold,
			AgentId:              agentID,
		}

		response, err := server.AnalyzeMongoLog(ctx, analyzeReq)
		if err != nil {
			httpStatus := http.StatusInternalServerError
			message := "MongoDB log dosyası analiz edilirken bir hata oluştu: " + err.Error()

			if err == context.DeadlineExceeded {
				httpStatus = http.StatusGatewayTimeout
				message = "İstek zaman aşımına uğradı"
			} else if st, ok := grpcstatus.FromError(err); ok {
				if st.Code() == codes.NotFound {
					c.JSON(http.StatusNotFound, gin.H{
						"status": "error",
						"error":  st.Message(),
					})
					return
				}
				message = st.Message()
			}

			c.JSON(httpStatus, gin.H{
				"status": "error",
				"error":  message,
			})
			return
		}

		// Log girdilerini daha okunabilir hale getir
		logEntries := make([]map[string]interface{}, 0, len(response.LogEntries))
		for _, entry := range response.LogEntries {
			logEntries = append(logEntries, map[string]interface{}{
				"timestamp":          entry.Timestamp,
				"timestamp_readable": time.Unix(entry.Timestamp, 0).Format(time.RFC3339),
				"severity":           entry.Severity,
				"component":          entry.Component,
				"context":            entry.Context,
				"message":            entry.Message,
				"db_name":            entry.DbName,
				"duration_millis":    entry.DurationMillis,
				"command":            entry.Command,
				"plan_summary":       entry.PlanSummary,
				"namespace":          entry.Namespace,
			})
		}

		// Başarılı yanıt
		c.JSON(http.StatusOK, gin.H{
			"status":       "success",
			"agent_id":     agentID,
			"log_path":     req.LogFilePath,
			"threshold_ms": req.SlowQueryThreshold,
			"log_entries":  logEntries,
		})
	}
}

// analyzePostgresLog, belirtilen agent'tan PostgreSQL log dosyasını analiz etmesini ister
func analyzePostgresLog(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Param("agent_id")

		if agentID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "agent_id parametresi gerekli",
			})
			return
		}

		var req struct {
			LogFilePath        string `json:"log_file_path" binding:"required"`
			SlowQueryThreshold int64  `json:"slow_query_threshold_ms"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Geçersiz JSON verisi: " + err.Error(),
			})
			return
		}

		// Context oluştur ve agent_id ekle
		ctx := context.WithValue(c.Request.Context(), "agent_id", agentID)
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()

		// PostgreSQL log dosyasını analiz et
		analyzeReq := &pb.PostgresLogAnalyzeRequest{
			LogFilePath:          req.LogFilePath,
			SlowQueryThresholdMs: req.SlowQueryThreshold,
			AgentId:              agentID,
		}

		response, err := server.AnalyzePostgresLog(ctx, analyzeReq)
		if err != nil {
			httpStatus := http.StatusInternalServerError
			message := "PostgreSQL log dosyası analiz edilirken bir hata oluştu: " + err.Error()

			if err == context.DeadlineExceeded {
				httpStatus = http.StatusGatewayTimeout
				message = "İstek zaman aşımına uğradı"
			} else if st, ok := grpcstatus.FromError(err); ok {
				if st.Code() == codes.NotFound {
					c.JSON(http.StatusNotFound, gin.H{
						"status": "error",
						"error":  st.Message(),
					})
					return
				}
				message = st.Message()
			}

			c.JSON(httpStatus, gin.H{
				"status": "error",
				"error":  message,
			})
			return
		}

		// Log girdilerini daha okunabilir hale getir
		logEntries := make([]map[string]interface{}, 0, len(response.LogEntries))
		for _, entry := range response.LogEntries {
			logEntries = append(logEntries, map[string]interface{}{
				"timestamp":              entry.Timestamp,
				"timestamp_readable":     time.Unix(entry.Timestamp, 0).Format(time.RFC3339),
				"log_level":              entry.LogLevel,
				"user_name":              entry.UserName,
				"database":               entry.Database,
				"process_id":             entry.ProcessId,
				"connection_from":        entry.ConnectionFrom,
				"session_id":             entry.SessionId,
				"session_line_num":       entry.SessionLineNum,
				"command_tag":            entry.CommandTag,
				"session_start_time":     entry.SessionStartTime,
				"virtual_transaction_id": entry.VirtualTransactionId,
				"transaction_id":         entry.TransactionId,
				"error_severity":         entry.ErrorSeverity,
				"sql_state_code":         entry.SqlStateCode,
				"message":                entry.Message,
				"detail":                 entry.Detail,
				"hint":                   entry.Hint,
				"internal_query":         entry.InternalQuery,
				"duration_ms":            entry.DurationMs,
			})
		}

		// Başarılı yanıt
		c.JSON(http.StatusOK, gin.H{
			"status":       "success",
			"agent_id":     agentID,
			"log_path":     req.LogFilePath,
			"threshold_ms": req.SlowQueryThreshold,
			"log_entries":  logEntries,
		})
	}
}

// formatBytes, bayt cinsinden boyutu okunabilir formata dönüştürür (KB, MB, GB)
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// getNodesHealth, tüm node sağlık bilgilerini birleştirip döndürür
func getNodesHealth(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		db := server.GetDB()
		// PostgreSQL verisini al
		postgresRows, err := db.Query("SELECT json_agg(sub.jsondata) FROM (SELECT jsondata FROM postgres_data ORDER BY id) AS sub")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch PostgreSQL data: " + err.Error()})
			return
		}
		defer postgresRows.Close()

		var postgresData []byte
		if postgresRows.Next() {
			err := postgresRows.Scan(&postgresData)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse PostgreSQL data: " + err.Error()})
				return
			}
		}

		// MongoDB verisini al
		mongoRows, err := db.Query("SELECT json_agg(sub.jsondata) FROM (SELECT jsondata FROM mongo_data ORDER BY id) AS sub")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch MongoDB data: " + err.Error()})
			return
		}
		defer mongoRows.Close()

		var mongoData []byte
		if mongoRows.Next() {
			err := mongoRows.Scan(&mongoData)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse MongoDB data: " + err.Error()})
				return
			}
		}

		// MSSQL verisini al
		mssqlRows, err := db.Query("SELECT json_agg(sub.jsondata) FROM (SELECT jsondata FROM mssql_data ORDER BY id) AS sub")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch MSSQL data: " + err.Error()})
			return
		}
		defer mssqlRows.Close()

		var mssqlData []byte
		if mssqlRows.Next() {
			err := mssqlRows.Scan(&mssqlData)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse MSSQL data: " + err.Error()})
				return
			}
		}

		// Verileri tek bir JSON yapısında birleştir
		responseData := gin.H{
			"postgresql": json.RawMessage(postgresData), // PostgreSQL verisini raw JSON olarak ekle
			"mongodb":    json.RawMessage(mongoData),    // MongoDB verisini raw JSON olarak ekle
			"mssql":      json.RawMessage(mssqlData),    // MSSQL verisini raw JSON olarak ekle
		}

		// Birleştirilmiş JSON verisini döndür
		c.JSON(http.StatusOK, responseData)
	}
}

// getAlarms, alarm listesini getirir
func getAlarms(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		// Query parametrelerini parse et
		onlyUnacknowledged := c.DefaultQuery("unacknowledged", "false") == "true"

		// Pagination parametreleri
		limit := 250 // Varsayılan limit
		if l := c.Query("limit"); l != "" {
			if parsedLimit, err := strconv.Atoi(l); err == nil && parsedLimit > 0 && parsedLimit <= 1000 {
				limit = parsedLimit
			}
		}

		offset := 0
		if o := c.Query("offset"); o != "" {
			if parsedOffset, err := strconv.Atoi(o); err == nil && parsedOffset >= 0 {
				offset = parsedOffset
			}
		}

		// Sayfa bazlı pagination desteği
		if page := c.Query("page"); page != "" {
			if parsedPage, err := strconv.Atoi(page); err == nil && parsedPage > 0 {
				offset = (parsedPage - 1) * limit
			}
		}

		// Filtering parametreleri
		severityFilter := c.Query("severity") // "critical", "warning", "info"
		metricFilter := c.Query("metric")     // metric_name filtresi

		// Tarih filtreleri
		var dateFrom, dateTo *time.Time
		if from := c.Query("date_from"); from != "" {
			if parsedFrom, err := time.Parse("2006-01-02", from); err == nil {
				dateFrom = &parsedFrom
			} else if parsedFrom, err := time.Parse(time.RFC3339, from); err == nil {
				dateFrom = &parsedFrom
			}
		}
		if to := c.Query("date_to"); to != "" {
			if parsedTo, err := time.Parse("2006-01-02", to); err == nil {
				// Günün sonuna ayarla
				endOfDay := parsedTo.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
				dateTo = &endOfDay
			} else if parsedTo, err := time.Parse(time.RFC3339, to); err == nil {
				dateTo = &parsedTo
			}
		}

		log.Printf("Alarm sorgusu parametreleri - Limit: %d, Offset: %d, Unacknowledged: %t, Severity: %s, Metric: %s",
			limit, offset, onlyUnacknowledged, severityFilter, metricFilter)

		alarms, totalCount, err := server.GetAlarms(ctx, onlyUnacknowledged, limit, offset, severityFilter, metricFilter, dateFrom, dateTo)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "Alarm verileri alınamadı: " + err.Error(),
			})
			return
		}

		// Pagination bilgilerini hesapla
		totalPages := int((totalCount + int64(limit) - 1) / int64(limit)) // Yukarı yuvarlama
		currentPage := (offset / limit) + 1

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data": gin.H{
				"alarms": alarms,
				"pagination": gin.H{
					"total_count":  totalCount,
					"current_page": currentPage,
					"total_pages":  totalPages,
					"limit":        limit,
					"offset":       offset,
					"has_next":     currentPage < totalPages,
					"has_previous": currentPage > 1,
				},
			},
		})
	}
}

// acknowledgeAlarm, belirtilen event_id'ye sahip alarmı acknowledge eder
func acknowledgeAlarm(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		eventID := c.Param("event_id")
		if eventID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "event_id parametresi gerekli",
			})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		err := server.AcknowledgeAlarm(ctx, eventID)
		if err != nil {
			status := http.StatusInternalServerError
			if err.Error() == fmt.Sprintf("belirtilen event_id ile alarm bulunamadı: %s", eventID) {
				status = http.StatusNotFound
			}

			c.JSON(status, gin.H{
				"status": "error",
				"error":  err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":  "success",
			"message": "Alarm başarıyla acknowledge edildi",
		})
	}
}

// readPostgresConfig, belirtilen agent'tan PostgreSQL config dosyasını okumasını ister
func readPostgresConfig(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Param("agent_id")

		if agentID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "agent_id parametresi gerekli",
			})
			return
		}

		var req struct {
			ConfigPath string `json:"config_path" binding:"required"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Geçersiz istek formatı: " + err.Error(),
			})
			return
		}

		// Context oluştur ve zaman aşımı ayarla
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		// PostgreSQL config dosyasını oku
		configReq := &pb.PostgresConfigRequest{
			AgentId:    agentID,
			ConfigPath: req.ConfigPath,
		}

		response, err := server.ReadPostgresConfig(ctx, configReq)
		if err != nil {
			httpStatus := http.StatusInternalServerError
			message := "PostgreSQL config dosyası okunurken bir hata oluştu: " + err.Error()

			if err == context.DeadlineExceeded {
				httpStatus = http.StatusGatewayTimeout
				message = "İstek zaman aşımına uğradı"
			} else if st, ok := grpcstatus.FromError(err); ok {
				if st.Code() == codes.NotFound {
					c.JSON(http.StatusNotFound, gin.H{
						"status": "error",
						"error":  st.Message(),
					})
					return
				}
				message = st.Message()
			}

			c.JSON(httpStatus, gin.H{
				"status": "error",
				"error":  message,
			})
			return
		}

		// Yapılandırma girişlerini daha okunabilir hale getir
		configEntries := make([]map[string]interface{}, 0, len(response.Configurations))
		for _, entry := range response.Configurations {
			configEntries = append(configEntries, map[string]interface{}{
				"parameter":   entry.Parameter,
				"value":       entry.Value,
				"description": entry.Description,
				"is_default":  entry.IsDefault,
				"category":    entry.Category,
			})
		}

		// Başarılı yanıt
		c.JSON(http.StatusOK, gin.H{
			"status":         "success",
			"agent_id":       agentID,
			"config_path":    response.ConfigPath,
			"configurations": configEntries,
		})
	}
}

// getAgentVersions, agent versiyon bilgilerini getirir
func getAgentVersions(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		versions, err := server.GetAgentVersions(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "Agent versiyon bilgileri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data": gin.H{
				"versions": versions,
			},
		})
	}
}

// GetUser, belirli bir kullanıcıyı ID'sine göre getirir
func GetUser(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.Param("id")

		// Kullanıcıyı veritabanından çek
		var user struct {
			ID        int       `json:"id"`
			Username  string    `json:"username"`
			Email     string    `json:"email"`
			Role      string    `json:"role"`
			CreatedAt time.Time `json:"created_at"`
			UpdatedAt time.Time `json:"updated_at"`
		}

		err := db.QueryRow(`
			SELECT id, username, email, role, created_at, updated_at 
			FROM users 
			WHERE id = $1`, userID).Scan(
			&user.ID, &user.Username, &user.Email, &user.Role,
			&user.CreatedAt, &user.UpdatedAt,
		)

		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{
				"status": "error",
				"error":  "Kullanıcı bulunamadı",
			})
			return
		}

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "Kullanıcı bilgileri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data": gin.H{
				"user": user,
			},
		})
	}
}

// promoteMongoToPrimary, MongoDB node'unu primary'ye yükseltir
func promoteMongoToPrimary(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			AgentID      string `json:"agent_id" binding:"required"`
			NodeHostname string `json:"node_hostname" binding:"required"`
			Port         int32  `json:"port" binding:"required"`
			ReplicaSet   string `json:"replica_set" binding:"required"`
			NodeStatus   string `json:"node_status" binding:"required"` // primary veya secondary
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Geçersiz istek formatı: " + err.Error(),
			})
			return
		}

		// Node status kontrolü
		if req.NodeStatus != "PRIMARY" && req.NodeStatus != "SECONDARY" {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "node_status 'primary' veya 'secondary' olmalıdır",
			})
			return
		}

		// Job ID oluştur
		jobID := uuid.New().String()

		// gRPC isteği oluştur
		grpcReq := &pb.MongoPromotePrimaryRequest{
			JobId:        jobID,
			AgentId:      req.AgentID,
			NodeHostname: req.NodeHostname,
			Port:         req.Port,
			ReplicaSet:   req.ReplicaSet,
			NodeStatus:   req.NodeStatus, // node durumunu ekle
		}

		// Context oluştur
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		// Promote işlemini başlat
		response, err := server.PromoteMongoToPrimary(ctx, grpcReq)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "Primary promotion başlatılamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusAccepted, gin.H{
			"status": "success",
			"data": gin.H{
				"job_id": response.JobId,
				"status": response.Status.String(),
			},
		})
	}
}

// promotePostgresToMaster, PostgreSQL node'unu master'a yükseltir
func promotePostgresToMaster(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			AgentID           string `json:"agent_id" binding:"required"`
			NodeHostname      string `json:"node_hostname" binding:"required"`
			DataDirectory     string `json:"data_directory" binding:"required"`
			CurrentMasterHost string `json:"current_master_host"` // Eski master bilgisi
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Geçersiz istek formatı: " + err.Error(),
			})
			return
		}

		// Job ID oluştur
		jobID := uuid.New().String()

		// gRPC isteği oluştur
		grpcReq := &pb.PostgresPromoteMasterRequest{
			JobId:             jobID,
			AgentId:           req.AgentID,
			NodeHostname:      req.NodeHostname,
			DataDirectory:     req.DataDirectory,
			CurrentMasterHost: req.CurrentMasterHost, // Eski master bilgisini ekle
		}

		// Context oluştur
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		// Promote işlemini başlat
		response, err := server.PromotePostgresToMaster(ctx, grpcReq)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "Master promotion başlatılamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusAccepted, gin.H{
			"status": "success",
			"data": gin.H{
				"job_id": response.JobId,
				"status": response.Status.String(),
			},
		})
	}
}

// convertPostgresToSlave, PostgreSQL master'ı slave'e dönüştürür
func convertPostgresToSlave(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			AgentID             string `json:"agent_id" binding:"required"`
			NodeHostname        string `json:"node_hostname" binding:"required"`
			NewMasterHost       string `json:"new_master_host" binding:"required"`
			NewMasterPort       int32  `json:"new_master_port"`
			DataDirectory       string `json:"data_directory" binding:"required"`
			ReplicationUser     string `json:"replication_user" binding:"required"`
			ReplicationPassword string `json:"replication_password" binding:"required"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Geçersiz istek formatı: " + err.Error(),
			})
			return
		}

		// Varsayılan port ayarla
		if req.NewMasterPort == 0 {
			req.NewMasterPort = 5432
		}

		// Job ID oluştur
		jobID := uuid.New().String()

		// İlk protobuf message'ları henüz generate edilmediği için geçici olarak normal job oluştur
		// gRPC isteği simüle et
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		// Job'ı oluştur
		job := &pb.Job{
			JobId:     jobID,
			Type:      pb.JobType_JOB_TYPE_POSTGRES_CONVERT_TO_SLAVE,
			Status:    pb.JobStatus_JOB_STATUS_PENDING,
			AgentId:   req.AgentID,
			CreatedAt: timestamppb.Now(),
			UpdatedAt: timestamppb.Now(),
			Parameters: map[string]string{
				"node_hostname":        req.NodeHostname,
				"new_master_host":      req.NewMasterHost,
				"new_master_port":      fmt.Sprintf("%d", req.NewMasterPort),
				"data_directory":       req.DataDirectory,
				"replication_user":     req.ReplicationUser,
				"replication_password": req.ReplicationPassword,
			},
		}

		// Job'ı server'a kaydet
		err := server.CreateJob(ctx, job)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "PostgreSQL slave dönüştürme işlemi başlatılamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusAccepted, gin.H{
			"status": "success",
			"data": gin.H{
				"job_id": jobID,
				"status": pb.JobStatus_JOB_STATUS_PENDING.String(),
			},
		})
	}
}

// getJob, belirli bir job'ın detaylarını getirir
func getJob(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		jobID := c.Param("job_id")
		if jobID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "job_id parametresi gerekli",
			})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		response, err := server.GetJob(ctx, &pb.GetJobRequest{JobId: jobID})
		if err != nil {
			status := http.StatusInternalServerError
			if err.Error() == fmt.Sprintf("job bulunamadı: %s", jobID) {
				status = http.StatusNotFound
			}

			c.JSON(status, gin.H{
				"status": "error",
				"error":  err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data": gin.H{
				"job": response.Job,
			},
		})
	}
}

// listJobs, job listesini getirir
func listJobs(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Printf("[DEBUG] listJobs API handler çağrıldı - URL: %s", c.Request.URL.String())

		// Query parametrelerini al
		agentID := c.Query("agent_id")
		status := c.Query("status")
		jobType := c.Query("type")
		limit := c.DefaultQuery("limit", "10")
		offset := c.DefaultQuery("offset", "0")

		log.Printf("[DEBUG] Query parametreleri: agent_id=%s, status=%s, type=%s, limit=%s, offset=%s",
			agentID, status, jobType, limit, offset)

		// Limit ve offset'i parse et
		limitInt, _ := strconv.ParseInt(limit, 10, 32)
		offsetInt, _ := strconv.ParseInt(offset, 10, 32)

		// gRPC isteği oluştur
		req := &pb.ListJobsRequest{
			AgentId: agentID,
			Limit:   int32(limitInt),
			Offset:  int32(offsetInt),
		}

		// Status parse et
		if status != "" {
			if val, ok := pb.JobStatus_value[status]; ok {
				req.Status = pb.JobStatus(val)
			} else {
				log.Printf("[DEBUG] Geçersiz status değeri: %s", status)
			}
		}

		// Type parse et
		if jobType != "" {
			if val, ok := pb.JobType_value[jobType]; ok {
				req.Type = pb.JobType(val)
			} else {
				log.Printf("[DEBUG] Geçersiz type değeri: %s", jobType)
			}
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		log.Printf("[DEBUG] ListJobs RPC çağrısı yapılıyor...")
		response, err := server.ListJobs(ctx, req)

		if err != nil {
			log.Printf("[ERROR] Job listesi alınamadı: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "Job listesi alınamadı: " + err.Error(),
			})
			return
		}

		log.Printf("[DEBUG] ListJobs RPC çağrısı başarılı - Dönen job sayısı: %d, Toplam: %d",
			len(response.Jobs), response.Total)

		// Dönen her job'ı logla
		for i, job := range response.Jobs {
			log.Printf("[DEBUG] Job[%d]: ID=%s, Agent=%s, Status=%s, Type=%s",
				i, job.JobId, job.AgentId, job.Status, job.Type)
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data": gin.H{
				"jobs":  response.Jobs,
				"total": response.Total,
			},
		})
	}
}

// createJob, genel bir job oluşturur
func createJob(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Type        string            `json:"type" binding:"required"`
			AgentID     string            `json:"agent_id" binding:"required"`
			Parameters  map[string]string `json:"parameters"`
			CustomJobID string            `json:"job_id"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Geçersiz istek formatı: " + err.Error(),
			})
			return
		}

		// Job tipini kontrol et
		jobType, ok := pb.JobType_value[req.Type]
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  fmt.Sprintf("Geçersiz job tipi: %s. Geçerli tipler: %v", req.Type, pb.JobType_name),
			})
			return
		}

		// Job ID oluştur (eğer özel ID verilmemişse)
		jobID := req.CustomJobID
		if jobID == "" {
			jobID = uuid.New().String()
		}

		// Context oluştur
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		// Job nesnesini oluştur
		job := &pb.Job{
			JobId:      jobID,
			Type:       pb.JobType(jobType),
			Status:     pb.JobStatus_JOB_STATUS_PENDING,
			AgentId:    req.AgentID,
			CreatedAt:  timestamppb.Now(),
			UpdatedAt:  timestamppb.Now(),
			Parameters: req.Parameters,
		}

		// Job'ı veritabanına kaydet
		if err := server.CreateJob(ctx, job); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "Job oluşturulamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"status": "success",
			"data": gin.H{
				"job_id": job.JobId,
				"type":   job.Type.String(),
				"status": job.Status.String(),
			},
		})
	}
}

// freezeMongoSecondary, MongoDB secondary node'larını belirli bir süre için dondurur
func freezeMongoSecondary(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			AgentID      string `json:"agent_id" binding:"required"`
			NodeHostname string `json:"node_hostname" binding:"required"`
			Port         int32  `json:"port" binding:"required"`
			ReplicaSet   string `json:"replica_set" binding:"required"`
			Seconds      int32  `json:"seconds"` // Opsiyonel, varsayılan 60
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Geçersiz istek formatı: " + err.Error(),
			})
			return
		}

		// Seconds parametresini kontrol et
		if req.Seconds <= 0 {
			req.Seconds = 60 // Varsayılan değer
		}

		// Job ID oluştur
		jobID := uuid.New().String()

		// gRPC isteği oluştur
		grpcReq := &pb.MongoFreezeSecondaryRequest{
			JobId:        jobID,
			AgentId:      req.AgentID,
			NodeHostname: req.NodeHostname,
			Port:         req.Port,
			ReplicaSet:   req.ReplicaSet,
			Seconds:      req.Seconds,
		}

		// Context oluştur
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		// Freeze işlemini başlat
		response, err := server.FreezeMongoSecondary(ctx, grpcReq)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "MongoDB secondary freeze işlemi başlatılamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusAccepted, gin.H{
			"status": "success",
			"data": gin.H{
				"job_id": response.JobId,
				"status": response.Status.String(),
			},
		})
	}
}

// explainQuery, PostgreSQL sorgusunun EXPLAIN ANALYZE planını çalıştırır
func explainQuery(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Param("agent_id")

		if agentID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "agent_id parametresi gerekli",
			})
			return
		}

		var req struct {
			Database string `json:"database" binding:"required"`
			Query    string `json:"query" binding:"required"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Geçersiz JSON verisi: " + err.Error(),
			})
			return
		}

		// Context oluştur
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		// Sorgu planını getir
		result, err := server.ExplainQuery(ctx, &pb.ExplainQueryRequest{
			AgentId:  agentID,
			Database: req.Database,
			Query:    req.Query,
		})

		if err != nil {
			status := http.StatusInternalServerError
			message := "Sorgu planı alınırken bir hata oluştu: " + err.Error()

			if err == context.DeadlineExceeded {
				status = http.StatusGatewayTimeout
				message = "Sorgu zaman aşımına uğradı"
			} else if strings.Contains(err.Error(), "agent bulunamadı") {
				status = http.StatusNotFound
				message = "Agent bulunamadı veya bağlantı kapalı"
			}

			c.JSON(status, gin.H{
				"status": "error",
				"error":  message,
			})
			return
		}

		// Başarılı yanıt
		if result.Status == "success" {
			c.JSON(http.StatusOK, gin.H{
				"status":   "success",
				"agent_id": agentID,
				"query":    req.Query,
				"database": req.Database,
				"plan":     result.Plan,
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  result.ErrorMessage,
			})
		}
	}
}

// explainMongoQuery, MongoDB sorgusunun explain planını çalıştırır
func explainMongoQuery(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Param("agent_id")

		if agentID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "agent_id parametresi gerekli",
			})
			return
		}

		// İstek body'sini raw olarak oku
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			log.Printf("[ERROR] İstek body'si okunamadı: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Sorgu okunamadı: " + err.Error(),
			})
			return
		}

		// Body'yi loglayalım
		log.Printf("[DEBUG] MongoDB explain ham istek (JSON): %s", string(bodyBytes))

		// Body'yi yeniden kullanmak için geri koy
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		var req struct {
			Database string `json:"database" binding:"required"`
			Query    string `json:"query" binding:"required"`
		}

		// JSON request'i parse et - temel validasyon için
		if err := c.ShouldBindJSON(&req); err != nil {
			log.Printf("[ERROR] JSON parse hatası: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Geçersiz JSON verisi: " + err.Error(),
			})
			return
		}

		log.Printf("[DEBUG] MongoDB explain istek bilgileri: agent_id=%s, database=%s", agentID, req.Database)

		// Context timeout ayarlama
		ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
		defer cancel()

		// Ham JSON sorguyu string olarak kullan
		queryStr := string(bodyBytes)
		log.Printf("[DEBUG] Server'a iletilen ham sorgu (karakter sayısı: %d)", len(queryStr))

		// ExplainMongoQuery servis metodunu çağır
		response, err := server.ExplainMongoQuery(ctx, &pb.ExplainQueryRequest{
			AgentId:  agentID,
			Database: req.Database,
			Query:    queryStr,
		})

		if err != nil {
			log.Printf("[ERROR] MongoDB sorgu planı alınırken bir hata oluştu: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "MongoDB sorgu planı alınırken bir hata oluştu: " + err.Error(),
			})
			return
		}

		if response.Status == "error" {
			log.Printf("[ERROR] MongoDB sorgu planı hatası: %s", response.ErrorMessage)
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  response.ErrorMessage,
			})
			return
		}

		// Başarılı cevap - Tekrar önemli değişiklik yapıldı
		log.Printf("[INFO] MongoDB explain başarılı, plan uzunluğu: %d karakter", len(response.Plan))

		// Plan direkt bir JSON objesi mi yoksa string mi kontrolü yap
		var jsonData interface{}
		if err := json.Unmarshal([]byte(response.Plan), &jsonData); err == nil {
			// Doğrudan JSON ise, yazma işlemi için Writer'a yaz
			c.Writer.Header().Set("Content-Type", "application/json")
			c.Writer.WriteHeader(http.StatusOK)
			_, err = c.Writer.Write([]byte(response.Plan))
			if err != nil {
				log.Printf("[ERROR] JSON yanıt yazılırken hata: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{
					"status": "error",
					"error":  "Yanıt yazılırken hata oluştu",
				})
			}
		} else {
			// JSON değilse, normal yanıt formatında döndür
			c.JSON(http.StatusOK, gin.H{
				"status": "success",
				"plan":   response.Plan,
			})
		}
	}
}

// explainMssqlQuery, MSSQL sorgusunun execution planını çalıştırır ve XML formatında döndürür
func explainMssqlQuery(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Param("agent_id")

		if agentID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "agent_id parametresi gerekli",
			})
			return
		}

		// İstek body'sini raw olarak oku
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			log.Printf("[ERROR] İstek body'si okunamadı: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Sorgu okunamadı: " + err.Error(),
			})
			return
		}

		// Body'yi loglayalım
		log.Printf("[DEBUG] MSSQL explain ham istek (JSON): %s", string(bodyBytes))

		// Body'yi yeniden kullanmak için geri koy
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		var req struct {
			Database string `json:"database" binding:"required"`
			Query    string `json:"query" binding:"required"`
		}

		// JSON request'i parse et - temel validasyon için
		if err := c.ShouldBindJSON(&req); err != nil {
			log.Printf("[ERROR] JSON parse hatası: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Geçersiz JSON verisi: " + err.Error(),
			})
			return
		}

		log.Printf("[DEBUG] MSSQL explain istek bilgileri: agent_id=%s, database=%s", agentID, req.Database)

		// Context timeout ayarlama
		ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
		defer cancel()

		// Ham JSON sorguyu string olarak kullan
		queryStr := string(bodyBytes)
		log.Printf("[DEBUG] Server'a iletilen ham sorgu (karakter sayısı: %d)", len(queryStr))

		// Unique bir sorgu ID'si oluştur
		queryID := fmt.Sprintf("mssql_explain_%d", time.Now().UnixNano())

		// MSSQL_EXPLAIN| formatında komut oluştur
		command := fmt.Sprintf("MSSQL_EXPLAIN|%s|%s", req.Database, req.Query)

		// Sorguyu agent'a gönder
		result, err := server.SendQuery(ctx, agentID, queryID, command, "")

		if err != nil {
			log.Printf("[ERROR] MSSQL sorgu planı alınırken bir hata oluştu: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "MSSQL sorgu planı alınırken bir hata oluştu: " + err.Error(),
			})
			return
		}

		if result.Result == nil {
			log.Printf("[ERROR] MSSQL sorgu planı boş döndü")
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "MSSQL sorgu planı boş döndü",
			})
			return
		}

		// Agent'dan dönen sonuç XML formatında olacak
		var resultValue string

		// Result.Result'ı XML string'e dönüştür
		if result.Result.TypeUrl == "type.googleapis.com/google.protobuf.Value" {
			// Value tipinde olanlar için direkt string değerini kullan
			resultValue = string(result.Result.Value)
			log.Printf("[DEBUG] Value tipi veri direkt string olarak kullanıldı, uzunluk: %d", len(resultValue))
		} else {
			// Struct veya diğer tipler için unmarshalling yapılmalı
			var resultStruct structpb.Struct
			if err := result.Result.UnmarshalTo(&resultStruct); err != nil {
				log.Printf("[ERROR] MSSQL sonucu ayrıştırılamadı: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{
					"status": "error",
					"error":  fmt.Sprintf("MSSQL sorgu planı ayrıştırılamadı: %v", err),
				})
				return
			}

			// "plan" veya "result" anahtarından XML değerini al
			if planValue, ok := resultStruct.Fields["plan"]; ok && planValue.GetStringValue() != "" {
				resultValue = planValue.GetStringValue()
			} else if planValue, ok := resultStruct.Fields["result"]; ok && planValue.GetStringValue() != "" {
				resultValue = planValue.GetStringValue()
			} else {
				// Tüm struct'ı JSON olarak serialize et
				resultMap := resultStruct.AsMap()
				resultBytes, err := json.Marshal(resultMap)
				if err != nil {
					log.Printf("[ERROR] MSSQL sonucu JSON serileştirilirken hata: %v", err)
					c.JSON(http.StatusInternalServerError, gin.H{
						"status": "error",
						"error":  fmt.Sprintf("MSSQL JSON serileştirme hatası: %v", err),
					})
					return
				}
				resultValue = string(resultBytes)
			}
		}

		// XML formatında döndür
		if strings.HasPrefix(strings.TrimSpace(resultValue), "<") {
			// XML formatında doğrudan döndür
			c.Header("Content-Type", "application/xml")
			c.String(http.StatusOK, resultValue)
		} else {
			// XML değilse normal JSON yanıt olarak döndür
			c.JSON(http.StatusOK, gin.H{
				"status": "success",
				"plan":   resultValue,
			})
		}
	}
}

// getMSSQLStatus, MSSQL durum bilgilerini getirir
func getMSSQLStatus(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		result, err := server.GetStatusMSSQL(ctx, nil)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "MSSQL durum bilgileri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   result.AsInterface(),
		})
	}
}

// GetLicences, lisans bilgilerini veritabanından çeker ve döndürür
func GetLicences(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Lisans bilgilerini companies tablosundan çek ve agents tablosu ile join yap
		query := `
			SELECT c.id, c.company_name, c.agent_key, c.expiration_date, c.created_at, c.updated_at, 
				   a.hostname, a.agent_id
			FROM companies c
			LEFT JOIN agents a ON c.id = a.company_id
			ORDER BY c.id
		`

		rows, err := db.Query(query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "Lisans bilgileri alınamadı: " + err.Error(),
			})
			return
		}
		defer rows.Close()

		// Sonuçları saklamak için map yapısı
		licenses := make(map[int]map[string]interface{})

		// Sonuçları işle
		for rows.Next() {
			var (
				id             int
				companyName    string
				agentKey       string
				expirationDate time.Time
				createdAt      time.Time
				updatedAt      time.Time
				hostname       sql.NullString // NULL olabilir
				agentID        sql.NullString // NULL olabilir
			)

			if err := rows.Scan(&id, &companyName, &agentKey, &expirationDate, &createdAt, &updatedAt, &hostname, &agentID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"status": "error",
					"error":  "Kayıtlar okunamadı: " + err.Error(),
				})
				return
			}

			// Bu company için önceden bir giriş yapılmış mı kontrol et
			license, exists := licenses[id]
			if !exists {
				// Company için yeni bir map oluştur
				license = map[string]interface{}{
					"id":              id,
					"company_name":    companyName,
					"agent_key":       agentKey,
					"expiration_date": expirationDate.Format(time.RFC3339),
					"created_at":      createdAt.Format(time.RFC3339),
					"updated_at":      updatedAt.Format(time.RFC3339),
					"agents":          []map[string]string{},
				}
				licenses[id] = license
			}

			// Eğer agent bilgisi varsa ekle
			if hostname.Valid && agentID.Valid {
				agentList := license["agents"].([]map[string]string)
				agentList = append(agentList, map[string]string{
					"hostname": hostname.String,
					"agent_id": agentID.String,
				})
				license["agents"] = agentList
			}
		}

		// Map yapısını slice haline getir
		licenseList := make([]map[string]interface{}, 0, len(licenses))
		for _, license := range licenses {
			licenseList = append(licenseList, license)
		}

		// JSON formatında döndür
		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data": gin.H{
				"licenses": licenseList,
			},
		})
	}
}

// getMSSQLBestPracticesAnalysis, MSSQL best practice analizi gerçekleştirir
func getMSSQLBestPracticesAnalysis(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Param("agent_id")
		if agentID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "agent_id parametresi gerekli",
			})
			return
		}

		// İsteğe bağlı parametreler
		database := c.Query("database")
		serverName := c.Query("server")

		log.Printf("[INFO] MSSQL Best Practices analizi başlatılıyor - Agent ID: %s, Database: %s, Server: %s",
			agentID, database, serverName)

		// 60 saniyelik bir timeout belirle
		ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
		defer cancel()

		// İstek oluştur
		req := &pb.BestPracticesAnalysisRequest{
			AgentId:      agentID,
			DatabaseName: database,
			ServerName:   serverName,
		}

		// Server metodunu çağır
		response, err := server.GetBestPracticesAnalysis(ctx, req)
		if err != nil {
			log.Printf("[ERROR] MSSQL Best Practices analizi hatası: %v", err)

			// Context timeout ise 504 dön
			if ctx.Err() == context.DeadlineExceeded {
				c.JSON(http.StatusGatewayTimeout, gin.H{
					"status": "error",
					"error":  "Best practices analizi zaman aşımına uğradı",
				})
				return
			}

			// Diğer hatalar için 500 dön
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  fmt.Sprintf("Best practices analizi hatası: %v", err),
			})
			return
		}

		// Başarılı yanıt
		log.Printf("[INFO] MSSQL Best Practices analizi tamamlandı - Agent ID: %s, Analysis ID: %s",
			agentID, response.AnalysisId)

		// Sonuçları JSON olarak parse etmeye çalış
		var jsonData interface{}
		if err := json.Unmarshal(response.AnalysisResults, &jsonData); err != nil {
			// Parse edilemezse doğrudan string olarak gönder
			c.JSON(http.StatusOK, gin.H{
				"status":             "success",
				"analysis_id":        response.AnalysisId,
				"analysis_timestamp": response.AnalysisTimestamp,
				"results":            string(response.AnalysisResults),
			})
		} else {
			// JSON olarak başarıyla parse edildiyse
			c.JSON(http.StatusOK, gin.H{
				"status":             "success",
				"analysis_id":        response.AnalysisId,
				"analysis_timestamp": response.AnalysisTimestamp,
				"results":            jsonData,
			})
		}
	}
}

// getProcessLogs, işlem loglarını almak için Gin handler
func getProcessLogs(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		// İstek parametrelerini al
		processID := c.Query("process_id")
		agentID := c.Query("agent_id")

		// Process ID kontrol et
		if processID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "process_id parametresi gereklidir",
			})
			return
		}

		// GetProcessStatus çağrısı yap
		req := &pb.ProcessStatusRequest{
			ProcessId: processID,
			AgentId:   agentID,
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		resp, err := server.GetProcessStatus(ctx, req)
		if err != nil {
			log.Printf("Process log alma hatası: %v", err)

			// gRPC hata kodlarını HTTP hata kodlarına çevir
			httpStatus := http.StatusInternalServerError
			errorMessage := fmt.Sprintf("Process logları alınamadı: %v", err)

			if st, ok := grpcstatus.FromError(err); ok {
				switch st.Code() {
				case codes.NotFound:
					httpStatus = http.StatusNotFound
					errorMessage = fmt.Sprintf("Belirtilen işlem bulunamadı: %s", processID)
				case codes.DeadlineExceeded:
					httpStatus = http.StatusGatewayTimeout
					errorMessage = "İşlem logları alınırken zaman aşımı"
				}
			}

			c.JSON(httpStatus, gin.H{
				"status": "error",
				"error":  errorMessage,
			})
			return
		}

		// JSON yanıtı döndür
		c.JSON(http.StatusOK, gin.H{
			"status":         "success",
			"process_id":     resp.ProcessId,
			"agent_id":       agentID,
			"type":           resp.ProcessType,
			"process_status": resp.Status,
			"logs":           resp.LogMessages,
			"elapsed_s":      resp.ElapsedTimeS,
			"created_at":     resp.CreatedAt,
			"updated_at":     resp.UpdatedAt,
			"metadata":       resp.Metadata,
		})
	}
}

// getRecentAlarms, dashboard için optimize edilmiş son alarmları getirir
func getRecentAlarms(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second) // Kısa timeout
		defer cancel()

		// Query parametrelerini parse et
		onlyUnacknowledged := c.DefaultQuery("unacknowledged", "true") == "true"

		// Limit parametresi (dashboard için max 50)
		limit := 20 // Varsayılan limit
		if l := c.Query("limit"); l != "" {
			if parsedLimit, err := strconv.Atoi(l); err == nil && parsedLimit > 0 && parsedLimit <= 50 {
				limit = parsedLimit
			}
		}

		alarms, err := server.GetRecentAlarms(ctx, limit, onlyUnacknowledged)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "Recent alarm verileri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data": gin.H{
				"alarms": alarms,
				"count":  len(alarms),
			},
		})
	}
}

// queryMetrics, InfluxDB'den özel metrik sorguları yapar
func queryMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Query string `json:"query" binding:"required"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Geçersiz sorgu formatı: " + err.Error(),
			})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		// InfluxDB writer'dan sorgu yap
		influxWriter := server.GetInfluxWriter()
		if influxWriter == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "error",
				"error":  "InfluxDB servisi kullanılamıyor",
			})
			return
		}

		results, err := influxWriter.QueryMetrics(ctx, req.Query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "Metrik sorgusu başarısız: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getCPUMetrics, CPU metriklerini getirir
func getCPUMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Query parametreleri
		agentID := c.Query("agent_id")
		timeRange := c.DefaultQuery("range", "1h")

		// Yeni field adlandırma sistemi için güncellenmiş sorgu
		var query string
		if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement =~ /cpu_usage|cpu_load|mssql_system/) |> filter(fn: (r) => r._field == "cpu_usage" or r._field == "usage_percent" or r._field == "load_average" or r._field == "cpu_cores") |> filter(fn: (r) => r.agent_id =~ /^%s$/)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement =~ /cpu_usage|cpu_load|mssql_system/) |> filter(fn: (r) => r._field == "cpu_usage" or r._field == "usage_percent" or r._field == "load_average" or r._field == "cpu_cores")`, timeRange)
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
				"error":  "CPU metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getMemoryMetrics, Memory metriklerini getirir
func getMemoryMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		timeRange := c.DefaultQuery("range", "1h")

		// Yeni field adlandırma sistemi için güncellenmiş sorgu
		var query string
		if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement =~ /memory_usage|memory_info|mssql_system/) |> filter(fn: (r) => r._field == "memory_usage" or r._field == "usage_percent" or r._field == "total_bytes" or r._field == "used_bytes" or r._field == "available_bytes" or r._field == "total_memory" or r._field == "free_memory") |> filter(fn: (r) => r.agent_id =~ /^%s$/)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement =~ /memory_usage|memory_info|mssql_system/) |> filter(fn: (r) => r._field == "memory_usage" or r._field == "usage_percent" or r._field == "total_bytes" or r._field == "used_bytes" or r._field == "available_bytes" or r._field == "total_memory" or r._field == "free_memory")`, timeRange)
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
				"error":  "Memory metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getDiskMetrics, Disk metriklerini getirir
func getDiskMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		timeRange := c.DefaultQuery("range", "1h")

		// Yeni field adlandırma sistemi için güncellenmiş sorgu
		var query string
		if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement =~ /disk_usage|disk_info|mssql_system/) |> filter(fn: (r) => r._field == "free_disk" or r._field == "total_disk" or r._field == "usage_percent" or r._field == "total_bytes" or r._field == "used_bytes" or r._field == "available_bytes") |> filter(fn: (r) => r.agent_id =~ /^%s$/)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement =~ /disk_usage|disk_info|mssql_system/) |> filter(fn: (r) => r._field == "free_disk" or r._field == "total_disk" or r._field == "usage_percent" or r._field == "total_bytes" or r._field == "used_bytes" or r._field == "available_bytes")`, timeRange)
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
				"error":  "Disk metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getNetworkMetrics, Network metriklerini getirir
func getNetworkMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		timeRange := c.DefaultQuery("range", "1h")

		// Flux sorgusu oluştur - tek satırda
		var query string
		if agentID != "" {
			// Agent ID'yi regex pattern olarak kullan
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "network_io" or r._measurement == "network_packets") |> filter(fn: (r) => r.agent_id =~ /^%s$/)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "network_io" or r._measurement == "network_packets")`, timeRange)
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
				"error":  "Network metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getDatabaseMetrics, Database metriklerini getirir
func getDatabaseMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		dbType := c.Query("type") // postgresql, mongodb, mssql
		timeRange := c.DefaultQuery("range", "1h")

		var measurementFilter string
		if dbType != "" {
			measurementFilter = fmt.Sprintf(`|> filter(fn: (r) => r._measurement =~ /^%s_/)`, dbType)
		} else {
			measurementFilter = `|> filter(fn: (r) => r._measurement =~ /^(postgresql|mongodb|mssql)_/)`
		}

		var query string
		if agentID != "" {
			// Agent ID'yi regex pattern olarak kullan
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) %s |> filter(fn: (r) => r.agent_id =~ /^%s$/)`, timeRange, measurementFilter, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) %s`, timeRange, measurementFilter)
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
				"error":  "Database metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getDashboardMetrics, dashboard için optimize edilmiş metrikleri getirir
func getDashboardMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		timeRange := c.DefaultQuery("range", "1h")

		// Dashboard için temel metrikler - yeni field adlandırma sistemi
		queries := map[string]string{
			"cpu_usage":     fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement =~ /cpu_usage|mssql_system/) |> filter(fn: (r) => r._field == "cpu_usage" or r._field == "usage_percent") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange),
			"memory_usage":  fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement =~ /memory_usage|mssql_system/) |> filter(fn: (r) => r._field == "memory_usage" or r._field == "usage_percent") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange),
			"disk_usage":    fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement =~ /disk_usage|mssql_system/) |> filter(fn: (r) => r._field == "free_disk" or r._field == "total_disk" or r._field == "usage_percent") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange),
			"response_time": fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mssql_response_time" or (r._measurement == "mssql_system" and r._field == "response_time_ms")) |> filter(fn: (r) => r._field == "response_time_ms") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange),
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

		results := make(map[string]interface{})

		for metricName, query := range queries {
			data, err := influxWriter.QueryMetrics(ctx, query)
			if err != nil {
				log.Printf("[ERROR] Dashboard metrik sorgusu başarısız (%s): %v", metricName, err)
				results[metricName] = []interface{}{}
			} else {
				results[metricName] = data
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getAvailableFields, InfluxDB'de mevcut field'ları listeler (debug amaçlı)
func getAvailableFields(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		measurement := c.Query("measurement")
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if measurement != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "%s") |> distinct(column: "_field")`, timeRange, measurement)
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> distinct(column: "_field")`, timeRange)
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
				"error":  "Field listesi alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getAvailableMeasurements, InfluxDB'de mevcut measurement'ları listeler (debug amaçlı)
func getAvailableMeasurements(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		timeRange := c.DefaultQuery("range", "1h")

		query := fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> distinct(column: "_measurement")`, timeRange)

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
				"error":  "Measurement listesi alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getMSSQLCPUMetrics, MSSQL CPU metriklerini InfluxDB'den alır
func getMSSQLCPUMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		timeRange := c.DefaultQuery("range", "1h")

		if agentID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "agent_id parametresi gereklidir",
			})
			return
		}

		// MSSQL CPU metriklerini almak için özel sorgu
		query := fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mssql_system") |> filter(fn: (r) => r._field == "cpu_usage") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> sort(columns: ["_time"], desc: true)`, timeRange, regexp.QuoteMeta(agentID))

		ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
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
				"error":  "MSSQL CPU metrikleri alınamadı: " + err.Error(),
			})
			return
		}

		// En son değeri al
		var latestCPUUsage float64
		var latestTimestamp string
		if len(results) > 0 {
			if val, ok := results[0]["_value"]; ok {
				if cpuVal, ok := val.(float64); ok {
					latestCPUUsage = cpuVal
				}
			}
			if timestamp, ok := results[0]["_time"]; ok {
				if timeStr, ok := timestamp.(string); ok {
					latestTimestamp = timeStr
				}
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data": gin.H{
				"agent_id":         agentID,
				"latest_cpu_usage": latestCPUUsage,
				"latest_timestamp": latestTimestamp,
				"all_data":         results,
			},
		})
	}
}

// getMSSQLConnectionsMetrics, MSSQL bağlantı metriklerini getirir
func getMSSQLConnectionsMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mssql_connections") |> filter(fn: (r) => r._field == "active" or r._field == "idle" or r._field == "total") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mssql_connections") |> filter(fn: (r) => r._field == "active" or r._field == "idle" or r._field == "total") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
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
				"error":  "MSSQL bağlantı metriklerini alırken hata: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getMSSQLSystemMetrics, MSSQL sistem metriklerini getirir
func getMSSQLSystemMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mssql_system") |> filter(fn: (r) => r._field == "cpu_usage" or r._field == "cpu_cores" or r._field == "memory_usage" or r._field == "free_memory" or r._field == "free_disk" or r._field == "total_disk" or r._field == "total_memory") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mssql_system") |> filter(fn: (r) => r._field == "cpu_usage" or r._field == "cpu_cores" or r._field == "memory_usage" or r._field == "free_memory" or r._field == "free_disk" or r._field == "total_disk" or r._field == "total_memory") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
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
				"error":  "MSSQL sistem metriklerini alırken hata: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getMSSQLDatabaseMetrics, MSSQL veritabanı metriklerini getirir
func getMSSQLDatabaseMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		database := c.Query("database")
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if agentID != "" && database != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mssql_database") |> filter(fn: (r) => r._field == "data_size" or r._field == "log_size") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> filter(fn: (r) => r.database =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID), regexp.QuoteMeta(database))
		} else if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mssql_database") |> filter(fn: (r) => r._field == "data_size" or r._field == "log_size") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mssql_database") |> filter(fn: (r) => r._field == "data_size" or r._field == "log_size") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
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
				"error":  "MSSQL veritabanı metriklerini alırken hata: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getMSSQLBlockingMetrics, MSSQL blocking session metriklerini getirir
func getMSSQLBlockingMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mssql_blocking") |> filter(fn: (r) => r._field == "sessions") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mssql_blocking") |> filter(fn: (r) => r._field == "sessions") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
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
				"error":  "MSSQL blocking metriklerini alırken hata: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getMSSQLWaitMetrics, MSSQL wait statistics metriklerini getirir
func getMSSQLWaitMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		waitType := c.Query("wait_type")
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if agentID != "" && waitType != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mssql_waits") |> filter(fn: (r) => r._field == "tasks" or r._field == "time_ms") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> filter(fn: (r) => r.wait_type =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID), regexp.QuoteMeta(waitType))
		} else if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mssql_waits") |> filter(fn: (r) => r._field == "tasks" or r._field == "time_ms") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mssql_waits") |> filter(fn: (r) => r._field == "tasks" or r._field == "time_ms") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
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
				"error":  "MSSQL wait metriklerini alırken hata: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getMSSQLDeadlockMetrics, MSSQL deadlock metriklerini getirir
func getMSSQLDeadlockMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mssql_deadlocks") |> filter(fn: (r) => r._field == "total") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mssql_deadlocks") |> filter(fn: (r) => r._field == "total") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
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
				"error":  "MSSQL deadlock metriklerini alırken hata: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getMSSQLAllMetrics, tüm MSSQL metriklerini tek seferde getirir (dashboard için)
func getMSSQLAllMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		timeRange := c.DefaultQuery("range", "1h")

		if agentID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "agent_id parametresi gereklidir",
			})
			return
		}

		// Tüm MSSQL metriklerini tek sorguda al (yeni performance ve transactions measurement'ları dahil)
		query := fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement =~ /^mssql_(system|connections|database|blocking|waits|deadlocks|response_time|performance|transactions)$/) |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> filter(fn: (r) => r._field !~ /_description$/ and r._field !~ /_unit$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false) |> group(columns: ["_measurement", "_field"])`, timeRange, regexp.QuoteMeta(agentID))

		ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
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
				"error":  "MSSQL metriklerini alırken hata: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
		})
	}
}

// getMSSQLResponseTimeMetrics, MSSQL response time metriklerini getirir
func getMSSQLResponseTimeMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mssql_response_time" or (r._measurement == "mssql_system" and r._field == "response_time_ms")) |> filter(fn: (r) => r._field == "response_time_ms") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 1m, fn: mean, createEmpty: false) |> sort(columns: ["_time"], desc: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mssql_response_time" or (r._measurement == "mssql_system" and r._field == "response_time_ms")) |> filter(fn: (r) => r._field == "response_time_ms") |> aggregateWindow(every: 1m, fn: mean, createEmpty: false) |> sort(columns: ["_time"], desc: false)`, timeRange)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
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
				"error":  "MSSQL response time metriklerini alırken hata: " + err.Error(),
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

// getMSSQLPerformanceMetrics, MSSQL performance metriklerini getirir (raw count değerleri)
func getMSSQLPerformanceMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mssql_performance") |> filter(fn: (r) => r._field == "batch_requests_count" or r._field == "buffer_cache_hit_ratio" or r._field == "compilations_count" or r._field == "lazy_writes_count" or r._field == "lock_requests_count" or r._field == "lock_timeouts_count" or r._field == "lock_waits_count" or r._field == "page_life_expectancy" or r._field == "page_reads_count" or r._field == "page_writes_count" or r._field == "recompilations_count") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mssql_performance") |> filter(fn: (r) => r._field == "batch_requests_count" or r._field == "buffer_cache_hit_ratio" or r._field == "compilations_count" or r._field == "lazy_writes_count" or r._field == "lock_requests_count" or r._field == "lock_timeouts_count" or r._field == "lock_waits_count" or r._field == "page_life_expectancy" or r._field == "page_reads_count" or r._field == "page_writes_count" or r._field == "recompilations_count") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
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
				"error":  "MSSQL performance metriklerini alırken hata: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
			"note":   "Raw cumulative count değerleri (sistem başladığından beri toplam)",
		})
	}
}

// getMSSQLTransactionsMetrics, MSSQL transactions metriklerini getirir (raw count değerleri)
func getMSSQLTransactionsMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if agentID != "" {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mssql_transactions") |> filter(fn: (r) => r._field == "active_by_database" or r._field == "active_total" or r._field == "longest_running_time_seconds" or r._field == "count" or r._field == "tempdb_free_space_kb" or r._field == "update_conflict_ratio" or r._field == "version_cleanup_rate_kb_count" or r._field == "version_generation_rate_kb_count" or r._field == "version_store_unit_count" or r._field == "version_store_unit_creation" or r._field == "version_store_unit_truncation" or r._field == "write_count" or r._field == "xtp_aborted_by_user_count" or r._field == "xtp_aborted_count" or r._field == "xtp_commit_dependencies_count" or r._field == "xtp_created_count") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mssql_transactions") |> filter(fn: (r) => r._field == "active_by_database" or r._field == "active_total" or r._field == "longest_running_time_seconds" or r._field == "count" or r._field == "tempdb_free_space_kb" or r._field == "update_conflict_ratio" or r._field == "version_cleanup_rate_kb_count" or r._field == "version_generation_rate_kb_count" or r._field == "version_store_unit_count" or r._field == "version_store_unit_creation" or r._field == "version_store_unit_truncation" or r._field == "write_count" or r._field == "xtp_aborted_by_user_count" or r._field == "xtp_aborted_count" or r._field == "xtp_commit_dependencies_count" or r._field == "xtp_created_count") |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
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
				"error":  "MSSQL transactions metriklerini alırken hata: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
			"note":   "Raw cumulative count değerleri (sistem başladığından beri toplam)",
		})
	}
}

// getMSSQLPerformanceRateMetrics, MSSQL performance metriklerinin rate hesaplamasını yapar (per-second)
func getMSSQLPerformanceRateMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if agentID != "" {
			// Rate hesaplaması için derivative kullan - sadece count alanları için
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mssql_performance") |> filter(fn: (r) => r._field == "batch_requests_count" or r._field == "compilations_count" or r._field == "lazy_writes_count" or r._field == "lock_requests_count" or r._field == "lock_timeouts_count" or r._field == "lock_waits_count" or r._field == "page_reads_count" or r._field == "page_writes_count" or r._field == "recompilations_count") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> derivative(unit: 1s, nonNegative: true) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mssql_performance") |> filter(fn: (r) => r._field == "batch_requests_count" or r._field == "compilations_count" or r._field == "lazy_writes_count" or r._field == "lock_requests_count" or r._field == "lock_timeouts_count" or r._field == "lock_waits_count" or r._field == "page_reads_count" or r._field == "page_writes_count" or r._field == "recompilations_count") |> derivative(unit: 1s, nonNegative: true) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
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
				"error":  "MSSQL performance rate metriklerini alırken hata: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
			"note":   "Rate hesaplaması yapılmış değerler (per second)",
		})
	}
}

// getMSSQLTransactionsRateMetrics, MSSQL transactions metriklerinin rate hesaplamasını yapar (per-second)
func getMSSQLTransactionsRateMetrics(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Query("agent_id")
		timeRange := c.DefaultQuery("range", "1h")

		var query string
		if agentID != "" {
			// Rate hesaplaması için derivative kullan - sadece count alanları için
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mssql_transactions") |> filter(fn: (r) => r._field == "count" or r._field == "version_cleanup_rate_kb_count" or r._field == "version_generation_rate_kb_count" or r._field == "write_count" or r._field == "xtp_aborted_by_user_count" or r._field == "xtp_aborted_count" or r._field == "xtp_commit_dependencies_count" or r._field == "xtp_created_count") |> filter(fn: (r) => r.agent_id =~ /^%s$/) |> derivative(unit: 1s, nonNegative: true) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange, regexp.QuoteMeta(agentID))
		} else {
			query = fmt.Sprintf(`from(bucket: "clustereye") |> range(start: -%s) |> filter(fn: (r) => r._measurement == "mssql_transactions") |> filter(fn: (r) => r._field == "count" or r._field == "version_cleanup_rate_kb_count" or r._field == "version_generation_rate_kb_count" or r._field == "write_count" or r._field == "xtp_aborted_by_user_count" or r._field == "xtp_aborted_count" or r._field == "xtp_commit_dependencies_count" or r._field == "xtp_created_count") |> derivative(unit: 1s, nonNegative: true) |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)`, timeRange)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
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
				"error":  "MSSQL transactions rate metriklerini alırken hata: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   results,
			"note":   "Rate hesaplaması yapılmış değerler (per second)",
		})
	}
}

// getCoordinationStatus, mevcut coordination state'ini gösterir (debug amaçlı)
func getCoordinationStatus(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Coordination state'ini al
		coordinationKeys, activeJobs := server.GetCoordinationStatus()

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data": gin.H{
				"processed_coordinations":  coordinationKeys,
				"active_coordination_jobs": activeJobs,
				"total_processed_keys":     len(coordinationKeys),
				"total_active_jobs":        len(activeJobs),
			},
		})
	}
}

// cleanupCoordinationState, coordination state'ini temizler (admin amaçlı)
func cleanupCoordinationState(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Action string `json:"action" binding:"required"` // "cleanup_all", "cleanup_old", "cleanup_key"
			Key    string `json:"key"`                       // Specific key to cleanup (for "cleanup_key" action)
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Geçersiz istek formatı: " + err.Error(),
			})
			return
		}

		var cleanedCount int
		var err error

		switch req.Action {
		case "cleanup_all":
			// Tüm coordination state'ini temizle
			cleanedCount = server.CleanupAllCoordination()
		case "cleanup_old":
			// Sadece eski kayıtları temizle (10 dakikadan eski)
			cleanedCount = server.CleanupOldCoordination()
		case "cleanup_key":
			// Belirli bir key'i temizle
			if req.Key == "" {
				c.JSON(http.StatusBadRequest, gin.H{
					"status": "error",
					"error":  "Key parametresi gerekli",
				})
				return
			}
			success := server.CleanupCoordinationKey(req.Key)
			if success {
				cleanedCount = 1
			} else {
				c.JSON(http.StatusNotFound, gin.H{
					"status": "error",
					"error":  "Belirtilen key bulunamadı",
				})
				return
			}
		default:
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Geçersiz action. Kullanılabilir: cleanup_all, cleanup_old, cleanup_key",
			})
			return
		}

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "Cleanup işlemi sırasında hata: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data": gin.H{
				"action":        req.Action,
				"cleaned_count": cleanedCount,
				"message":       fmt.Sprintf("%d coordination key temizlendi", cleanedCount),
			},
		})
	}
}
