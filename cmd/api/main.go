package main

import (
	"context"
	"database/sql"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/sefaphlvn/clustereye-test/internal/api"
	"github.com/sefaphlvn/clustereye-test/internal/config"
	"github.com/sefaphlvn/clustereye-test/internal/database"
	"github.com/sefaphlvn/clustereye-test/internal/metrics"
	"github.com/sefaphlvn/clustereye-test/internal/server"
	pb "github.com/sefaphlvn/clustereye-test/pkg/agent"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
)

// AgentConnection, bağlı bir agent'ı temsil eder
type AgentConnection struct {
	stream pb.AgentService_ConnectServer
	info   *pb.AgentInfo
}

// QueryResponse, sorgu sonuçlarını temsil eder
type QueryResponse struct {
	Result     string
	ResultChan chan *pb.QueryResult
}

type Server struct {
	pb.UnimplementedAgentServiceServer
	mu          sync.RWMutex
	agents      map[string]*AgentConnection
	queryMu     sync.RWMutex
	queryResult map[string]*QueryResponse
	db          *sql.DB
	companyRepo *database.CompanyRepository
}

func NewServer(db *sql.DB) *Server {
	return &Server{
		agents:      make(map[string]*AgentConnection),
		queryResult: make(map[string]*QueryResponse),
		db:          db,
		companyRepo: database.NewCompanyRepository(db),
	}
}

func main() {
	// Konfigürasyon yükleniyor
	cfg, err := config.LoadServerConfig()
	if err != nil {
		log.Fatalf("Konfigürasyon yüklenemedi: %v", err)
	}

	// Veritabanı bağlantısı
	db, err := database.ConnectDatabase(cfg.Database)
	if err != nil {
		log.Fatalf("Veritabanı bağlantısı kurulamadı: %v", err)
	}
	defer db.Close()

	// InfluxDB Writer'ı başlat
	influxWriter, err := metrics.NewInfluxDBWriter(cfg.InfluxDB)
	if err != nil {
		log.Fatalf("InfluxDB bağlantısı kurulamadı: %v", err)
	}
	defer influxWriter.Close()

	// gRPC Server başlat
	listener, err := net.Listen("tcp", cfg.GRPC.Address)
	if err != nil {
		log.Fatal(err)
	}

	// gRPC sunucu seçeneklerini ayarla
	maxMsgSize := 128 * 1024 * 1024 // 128MB
	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(maxMsgSize),
		grpc.MaxSendMsgSize(maxMsgSize),
	}

	grpcServer := grpc.NewServer(opts...)
	serverInstance := server.NewServer(db, influxWriter)
	pb.RegisterAgentServiceServer(grpcServer, serverInstance)

	go func() {
		log.Printf("Cloud API gRPC server çalışıyor: %s", cfg.GRPC.Address)
		if err := grpcServer.Serve(listener); err != nil {
			log.Fatal(err)
		}
	}()

	// HTTP Gin API Server başlat
	router := gin.Default()

	// CORS middleware'ini yapılandır
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173", "http://localhost:3000", "http://localhost:8080", "https://senbaris.github.io", "https://clabapi.clustereye.com", "https://commercelab.clustereye.com"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "Cookie"},
		ExposeHeaders:    []string{"Content-Length", "Authorization"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// API handler'larını kaydet
	api.RegisterHandlers(router, serverInstance)

	// HTTP sunucusunu başlat
	log.Printf("HTTP API server çalışıyor: %s", cfg.HTTP.Address)
	if err := http.ListenAndServe(cfg.HTTP.Address, router); err != nil {
		log.Fatal(err)
	}
}

// Connect, agent'ların bağlanması için stream açar
func (s *Server) Connect(stream pb.AgentService_ConnectServer) error {
	var currentAgentID string
	var companyID int

	for {
		in, err := stream.Recv()
		if err != nil {
			log.Printf("Agent %s bağlantısı kapandı: %v", currentAgentID, err)
			s.mu.Lock()
			delete(s.agents, currentAgentID)
			s.mu.Unlock()
			return err
		}

		switch payload := in.Payload.(type) {
		case *pb.AgentMessage_AgentInfo:
			agentInfo := payload.AgentInfo

			// Agent anahtarını doğrula
			company, err := s.companyRepo.ValidateAgentKey(context.Background(), agentInfo.Key)
			if err != nil {
				// Hata durumunda agent'a bildir
				errMsg := "Geçersiz agent anahtarı"
				if err == database.ErrKeyExpired {
					errMsg = "Agent anahtarı süresi dolmuş"
				}

				// Hata mesajını agent'a gönder
				stream.Send(&pb.ServerMessage{
					Payload: &pb.ServerMessage_Error{
						Error: &pb.Error{
							Code:    "AUTH_ERROR",
							Message: errMsg,
						},
					},
				})

				log.Printf("Agent kimlik doğrulama hatası: %v", err)
				return err
			}

			// Agent ID'yi belirle
			currentAgentID = agentInfo.AgentId
			companyID = company.ID

			// Agent'ı kaydet
			err = s.companyRepo.RegisterAgent(
				context.Background(),
				companyID,
				currentAgentID,
				agentInfo.Hostname,
				agentInfo.Ip,
			)

			if err != nil {
				log.Printf("Agent kaydedilemedi: %v", err)
				stream.Send(&pb.ServerMessage{
					Payload: &pb.ServerMessage_Error{
						Error: &pb.Error{
							Code:    "REGISTRATION_ERROR",
							Message: "Agent kaydedilemedi",
						},
					},
				})
				return err
			}

			// Agent'ı bağlantı listesine ekle
			s.mu.Lock()
			s.agents[currentAgentID] = &AgentConnection{
				stream: stream,
				info:   agentInfo,
			}
			s.mu.Unlock()

			// Başarılı kayıt mesajı gönder
			stream.Send(&pb.ServerMessage{
				Payload: &pb.ServerMessage_Registration{
					Registration: &pb.RegistrationResult{
						Status:  "success",
						Message: "Agent başarıyla kaydedildi",
					},
				},
			})

			log.Printf("Yeni Agent bağlandı: %+v (Firma: %s)", agentInfo, company.CompanyName)

		case *pb.AgentMessage_QueryResult:
			// Mevcut sorgu sonucu işleme kodu...
		}
	}
}
