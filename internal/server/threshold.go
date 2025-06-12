package server

import (
	"context"
	"database/sql"

	"github.com/sefaphlvn/clustereye-test/internal/logger"
	pb "github.com/sefaphlvn/clustereye-test/pkg/agent"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GetThresholdSettings, agent'ın threshold ayarlarını almasını sağlar
func (s *Server) GetThresholdSettings(ctx context.Context, req *pb.GetThresholdSettingsRequest) (*pb.GetThresholdSettingsResponse, error) {
	logger.Info().Str("agent_id", req.AgentId).Msg("Threshold ayarları istendi")

	// Veritabanından threshold ayarlarını al
	var settings pb.ThresholdSettings
	err := s.db.QueryRowContext(ctx, `
        SELECT cpu_threshold, memory_threshold, disk_threshold, 
               slow_query_threshold_ms, connection_threshold, replication_lag_threshold,
               blocking_query_threshold_ms
        FROM threshold_settings 
        ORDER BY id DESC LIMIT 1
    `).Scan(
		&settings.CpuThreshold,
		&settings.MemoryThreshold,
		&settings.DiskThreshold,
		&settings.SlowQueryThresholdMs,
		&settings.ConnectionThreshold,
		&settings.ReplicationLagThreshold,
		&settings.BlockingQueryThresholdMs,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			// Eğer ayar bulunamazsa varsayılan değerleri döndür
			settings = pb.ThresholdSettings{
				CpuThreshold:             80.0, // %80
				MemoryThreshold:          80.0, // %80
				DiskThreshold:            85.0, // %85
				SlowQueryThresholdMs:     1000, // 1 saniye
				ConnectionThreshold:      100,  // 100 bağlantı
				ReplicationLagThreshold:  300,  // 300 saniye
				BlockingQueryThresholdMs: 1000, // 1 saniye
			}
			logger.Warn().Str("agent_id", req.AgentId).Msg("Threshold ayarları bulunamadı, varsayılan değerler kullanılıyor")
		} else {
			logger.Error().Err(err).Str("agent_id", req.AgentId).Msg("Threshold ayarları alınırken hata oluştu")
			return nil, status.Errorf(codes.Internal, "Threshold ayarları alınamadı: %v", err)
		}
	}

	logger.Info().Str("agent_id", req.AgentId).Msg("Threshold ayarları başarıyla gönderildi")
	return &pb.GetThresholdSettingsResponse{
		Settings: &settings,
	}, nil
}
