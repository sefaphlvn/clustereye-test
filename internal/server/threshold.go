package server

import (
	"context"
	"database/sql"
	"log"

	pb "github.com/sefaphlvn/clustereye-test/pkg/agent"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GetThresholdSettings, agent'ın threshold ayarlarını almasını sağlar
func (s *Server) GetThresholdSettings(ctx context.Context, req *pb.GetThresholdSettingsRequest) (*pb.GetThresholdSettingsResponse, error) {
	log.Printf("Threshold ayarları istendi - Agent ID: %s", req.AgentId)

	// Veritabanından threshold ayarlarını al
	var settings pb.ThresholdSettings
	err := s.db.QueryRowContext(ctx, `
        SELECT cpu_threshold, memory_threshold, disk_threshold, 
               slow_query_threshold_ms, connection_threshold, replication_lag_threshold
        FROM threshold_settings 
        ORDER BY id DESC LIMIT 1
    `).Scan(
		&settings.CpuThreshold,
		&settings.MemoryThreshold,
		&settings.DiskThreshold,
		&settings.SlowQueryThresholdMs,
		&settings.ConnectionThreshold,
		&settings.ReplicationLagThreshold,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			// Eğer ayar bulunamazsa varsayılan değerleri döndür
			settings = pb.ThresholdSettings{
				CpuThreshold:            80.0, // %80
				MemoryThreshold:         80.0, // %80
				DiskThreshold:           85.0, // %85
				SlowQueryThresholdMs:    1000, // 1 saniye
				ConnectionThreshold:     100,  // 100 bağlantı
				ReplicationLagThreshold: 300,  // 300 saniye
			}
			log.Printf("Threshold ayarları bulunamadı, varsayılan değerler kullanılıyor - Agent ID: %s", req.AgentId)
		} else {
			log.Printf("Threshold ayarları alınırken hata oluştu: %v - Agent ID: %s", err, req.AgentId)
			return nil, status.Errorf(codes.Internal, "Threshold ayarları alınamadı: %v", err)
		}
	}

	log.Printf("Threshold ayarları başarıyla gönderildi - Agent ID: %s", req.AgentId)
	return &pb.GetThresholdSettingsResponse{
		Settings: &settings,
	}, nil
}
