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
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sefaphlvn/clustereye-test/internal/server"
	pb "github.com/sefaphlvn/clustereye-test/pkg/agent"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
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
	}

	// Status Endpoint'leri
	status := v1.Group("/status")
	{
		// PostgreSQL durum bilgilerini getir
		status.GET("/postgres", getPostgresStatus(server))
		// MongoDB durum bilgilerini getir
		status.GET("/mongo", getMongoStatus(server))
		// Agent durum bilgilerini getir
		status.GET("/agents", getAgentStatus(server))
		// Tüm node sağlık bilgilerini getir
		status.GET("/nodeshealth", getNodesHealth(server))
		// Alarm listesini getir
		status.GET("/alarms", getAlarms(server))
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
		// Job durumunu sorgula
		jobs.GET("/:job_id", getJob(server))
		// Job listesini getir
		jobs.GET("", listJobs(server))
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
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
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
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
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

		// Verileri tek bir JSON yapısında birleştir
		responseData := gin.H{
			"postgresql": json.RawMessage(postgresData), // PostgreSQL verisini raw JSON olarak ekle
			"mongodb":    json.RawMessage(mongoData),    // MongoDB verisini raw JSON olarak ekle
		}

		// Birleştirilmiş JSON verisini döndür
		c.JSON(http.StatusOK, responseData)
	}
}

// getAlarms, alarm listesini getirir
func getAlarms(server *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		// Query parameter'dan sadece acknowledge edilmemiş alarmları getirip getirmeyeceğimizi kontrol et
		onlyUnacknowledged := c.DefaultQuery("unacknowledged", "true") == "false"

		alarms, err := server.GetAlarms(ctx, onlyUnacknowledged)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "Alarm verileri alınamadı: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data": gin.H{
				"alarms": alarms,
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
			AgentID       string `json:"agent_id" binding:"required"`
			NodeHostname  string `json:"node_hostname" binding:"required"`
			DataDirectory string `json:"data_directory" binding:"required"`
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
			JobId:         jobID,
			AgentId:       req.AgentID,
			NodeHostname:  req.NodeHostname,
			DataDirectory: req.DataDirectory,
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
		// Query parametrelerini al
		agentID := c.Query("agent_id")
		status := c.Query("status")
		jobType := c.Query("type")
		limit := c.DefaultQuery("limit", "10")
		offset := c.DefaultQuery("offset", "0")

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
			}
		}

		// Type parse et
		if jobType != "" {
			if val, ok := pb.JobType_value[jobType]; ok {
				req.Type = pb.JobType(val)
			}
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		response, err := server.ListJobs(ctx, req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "Job listesi alınamadı: " + err.Error(),
			})
			return
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
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
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

		// Başarılı cevap
		log.Printf("[INFO] MongoDB explain başarılı, plan uzunluğu: %d karakter", len(response.Plan))
		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"plan":   response.Plan,
		})
	}
}
