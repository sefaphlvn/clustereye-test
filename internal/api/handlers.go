package api

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sefaphlvn/clustereye-test/internal/server"
)

// RegisterHandlers, API rotalarını Gin router'a kaydeder
func RegisterHandlers(router *gin.Engine, server *server.Server) {
	// API grupları oluştur
	v1 := router.Group("/api/v1")

	// Agent Endpoint'leri
	agents := v1.Group("/agents")
	{
		// Tüm bağlı agent'ları listele
		agents.GET("", getAgents(server))

		// Agent'a sorgu gönder
		agents.POST("/:agent_id/query", sendQueryToAgent(server))
	}

	// Status Endpoint'leri
	status := v1.Group("/status")
	{
		// PostgreSQL durum bilgilerini getir
		status.GET("/postgres", getPostgresStatus(server))
		// Agent durum bilgilerini getir
		status.GET("/agents", getAgentStatus(server))
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
			QueryID string `json:"query_id" binding:"required"`
			Command string `json:"command" binding:"required"`
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
		result, err := server.SendQuery(ctx, agentID, req.QueryID, req.Command)
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
