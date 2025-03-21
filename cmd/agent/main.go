package main

import (
	"context"
	"log"
	"net"
	"os"
	"time"

	"github.com/sefaphlvn/clustereye-test/internal/config"
	pb "github.com/sefaphlvn/clustereye-test/pkg/agent"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"
)

func main() {
	// Konfigürasyonu yükle
	cfg, err := config.LoadAgentConfig()
	if err != nil {
		log.Fatalf("Konfigürasyon yüklenemedi: %v", err)
	}

	// GRPC Bağlantısı kur
	conn, err := grpc.Dial(cfg.GRPC.ServerAddress, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("gRPC sunucusuna bağlanılamadı: %v", err)
	}
	defer conn.Close()

	// gRPC client oluştur
	client := pb.NewAgentServiceClient(conn)
	stream, err := client.Connect(context.Background())
	if err != nil {
		log.Fatalf("Connect stream oluşturulamadı: %v", err)
	}

	// Sistem bilgilerini al
	hostname, _ := os.Hostname()
	ip := getLocalIP()
	
	// Kimlik doğrulama ayarlarını al
	postgreAuth := cfg.PostgreSQL.Auth
	
	// Test veritabanı bağlantısı
	testResult := testDBConnection(cfg)

	// Agent bilgilerini gönder
	agentInfo := &pb.AgentMessage{
		Payload: &pb.AgentMessage_AgentInfo{
			AgentInfo: &pb.AgentInfo{
				Key:      cfg.Key,
				AgentId:  "agent_" + hostname,
				Hostname: hostname,
				Ip:       ip,
				Auth:     postgreAuth,
				Test:     testResult,
			},
		},
	}

	if err := stream.Send(agentInfo); err != nil {
		log.Fatalf("Agent bilgisi gönderilemedi: %v", err)
	}

	log.Printf("ClusterEye sunucusuna bağlandı: %s", cfg.GRPC.ServerAddress)

	// Komut alma işlemi
	go func() {
		for {
			in, err := stream.Recv()
			if err != nil {
				log.Fatalf("Cloud API bağlantısı kapandı: %v", err)
			}

			if query := in.GetQuery(); query != nil {
				log.Printf("Yeni sorgu geldi: %s", query.Command)

				// Sorguyu işle ve sonucu hesapla
				queryResult := processQuery(query.Command)

				// Sorgu sonucunu hazırla
				resultMap, err := structpb.NewStruct(queryResult)
				if err != nil {
					log.Printf("Sonuç haritası oluşturulamadı: %v", err)
					continue
				}
				
				// structpb'yi Any'e dönüştür
				anyResult, err := anypb.New(resultMap)
				if err != nil {
					log.Printf("Any tipine dönüştürülemedi: %v", err)
					continue
				}

				// Sorgu sonucunu gönder
				result := &pb.AgentMessage{
					Payload: &pb.AgentMessage_QueryResult{
						QueryResult: &pb.QueryResult{
							QueryId: query.QueryId,
							Result:  anyResult,
						},
					},
				}

				if err := stream.Send(result); err != nil {
					log.Fatalf("Sorgu cevabı gönderilemedi: %v", err)
				}
			}
		}
	}()

	// Agent bağlantısını canlı tutmak için basit bir döngü
	for {
		time.Sleep(time.Minute)
	}
}

// processQuery, gelen sorguyu işler ve sonuç döndürür
func processQuery(command string) map[string]interface{} {
	// Bu basitçe bir test yanıtı, gerçek uygulamada burada komut çalıştırılabilir
	return map[string]interface{}{
		"status":  "success",
		"command": command,
		"result":  "Command executed successfully",
		"time":    time.Now().String(),
	}
}

// testDBConnection, konfigürasyondaki veritabanı bilgileriyle test bağlantısı yapar
func testDBConnection(cfg *config.AgentConfig) string {
	// Bu bir örnek fonksiyondur. Gerçek uygulamada burada PostgreSQL veya MongoDB
	// bağlantısı test edilebilir. Şimdilik başarılı döndürelim.
	return "success"
}

// getLocalIP, yerel IP'yi almak için yardımcı fonksiyon
func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
} 