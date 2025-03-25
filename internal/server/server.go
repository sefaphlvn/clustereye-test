package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/sefaphlvn/clustereye-test/internal/database"
	pb "github.com/sefaphlvn/clustereye-test/pkg/agent"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

// PostgresInfo, PostgreSQL test sonucunu ve bilgilerini temsil eder
type PostgresInfo struct {
	Status   string `json:"status"`
	User     string `json:"user"`
	Password string `json:"password"`
	Cluster  string `json:"cluster"`
}

// AgentConnection, bir agent ile olan bağlantıyı temsil eder
type AgentConnection struct {
	Stream pb.AgentService_ConnectServer
	Info   *pb.AgentInfo
}

// QueryResponse, bir sorgu sonucunu ve sonuç kanalını içerir
type QueryResponse struct {
	Result     string
	ResultChan chan *pb.QueryResult
}

// Server, ClusterEye sunucusunu temsil eder
type Server struct {
	pb.UnimplementedAgentServiceServer
	mu          sync.RWMutex
	agents      map[string]*AgentConnection
	queryMu     sync.RWMutex
	queryResult map[string]*QueryResponse // query_id -> QueryResponse
	db          *sql.DB                   // PostgreSQL veritabanı bağlantısı
	companyRepo *database.CompanyRepository
}

// NewServer, yeni bir sunucu nesnesi oluşturur
func NewServer(db *sql.DB) *Server {
	return &Server{
		agents:      make(map[string]*AgentConnection),
		queryResult: make(map[string]*QueryResponse),
		db:          db,
		companyRepo: database.NewCompanyRepository(db),
	}
}

// Connect, agent'ların bağlanması için kullanılan gRPC stream metodudur
func (s *Server) Connect(stream pb.AgentService_ConnectServer) error {
	var currentAgentID string

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
			companyID := company.ID // Kullanılmayan değişkeni düzelttik

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

			// PostgreSQL bağlantı bilgilerini kaydet
			log.Printf("PostgreSQL bilgileri kaydediliyor: hostname=%s, cluster=%s, user=%s",
				agentInfo.Hostname, agentInfo.Platform, agentInfo.PostgresUser)

			// Veritabanı bağlantısını kontrol et
			if err := s.checkDatabaseConnection(); err != nil {
				log.Printf("Veritabanı bağlantı hatası: %v", err)
			}

			// PostgreSQL bilgilerini kaydet
			err = s.companyRepo.SavePostgresConnInfo(
				context.Background(),
				agentInfo.Hostname,
				agentInfo.Platform,     // Platform alanını cluster adı olarak kullanıyoruz
				agentInfo.PostgresUser, // Agent'dan gelen kullanıcı adı
				agentInfo.PostgresPass, // Agent'dan gelen şifre
			)

			if err != nil {
				log.Printf("PostgreSQL bağlantı bilgileri kaydedilemedi: %v", err)
				// Hata detaylarını göster
				// pq paketi eksik olduğu için bu kısmı yorum satırına alıyoruz
				// if pgErr, ok := err.(*pq.Error); ok {
				//     log.Printf("PostgreSQL hata detayları: %+v", pgErr)
				// }
			} else {
				log.Printf("PostgreSQL bağlantı bilgileri kaydedildi: %s", agentInfo.Hostname)
			}

			// Agent'ı bağlantı listesine ekle
			s.mu.Lock()
			s.agents[currentAgentID] = &AgentConnection{
				Stream: stream,
				Info:   agentInfo,
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

			log.Printf("Yeni Agent bağlandı ve kaydedildi: %+v (Firma: %s)", agentInfo, company.CompanyName)

		case *pb.AgentMessage_QueryResult:
			queryResult := payload.QueryResult
			log.Printf("Agent %s sorguya cevap verdi: %+v", currentAgentID, queryResult)

			// Sorgu sonucunu ilgili kanal üzerinden ilet
			s.queryMu.RLock()
			queryResp, ok := s.queryResult[queryResult.QueryId]
			s.queryMu.RUnlock()

			if ok && queryResp.ResultChan != nil {
				queryResp.ResultChan <- queryResult
			}
		}
	}
}

// Register, agent'ın kaydı için kullanılan gRPC metodudur
func (s *Server) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	agentInfo := req.AgentInfo

	// Agent anahtarını doğrula
	company, err := s.companyRepo.ValidateAgentKey(ctx, agentInfo.Key)
	if err != nil {
		return &pb.RegisterResponse{
			Registration: &pb.RegistrationResult{
				Status:  "error",
				Message: "Geçersiz agent anahtarı",
			},
		}, nil
	}

	// Agent'ı kaydet
	err = s.companyRepo.RegisterAgent(
		ctx,
		company.ID,
		agentInfo.AgentId,
		agentInfo.Hostname,
		agentInfo.Ip,
	)

	if err != nil {
		return &pb.RegisterResponse{
			Registration: &pb.RegistrationResult{
				Status:  "error",
				Message: "Agent kaydedilemedi",
			},
		}, nil
	}

	return &pb.RegisterResponse{
		Registration: &pb.RegistrationResult{
			Status:  "success",
			Message: "Agent başarıyla kaydedildi",
		},
	}, nil
}

func (s *Server) ExecuteQuery(ctx context.Context, req *pb.QueryRequest) (*pb.QueryResponse, error) {
	// Sorgu işleme mantığı
	// ...
	return &pb.QueryResponse{
		Result: &pb.QueryResult{
			QueryId: req.Query.QueryId,
			// Sonuç verilerini doldurun
		},
	}, nil
}

func (s *Server) SendPostgresInfo(ctx context.Context, req *pb.PostgresInfoRequest) (*pb.PostgresInfoResponse, error) {
	// PostgreSQL bilgilerini işleme mantığı
	// ...
	return &pb.PostgresInfoResponse{
		Status: "success",
	}, nil
}

func (s *Server) StreamQueries(stream pb.AgentService_StreamQueriesServer) error {
	// Sürekli sorgu akışı mantığı
	// ...
	return nil
}

func (s *Server) StreamPostgresInfo(stream pb.AgentService_StreamPostgresInfoServer) error {
	// Sürekli PostgreSQL bilgi akışı mantığı
	// ...
	return nil
}

// GetStatusPostgres, PostgreSQL veritabanından durum bilgilerini çeker
func (s *Server) GetStatusPostgres(ctx context.Context, _ *structpb.Struct) (*structpb.Value, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT json_agg(sub.jsondata) FROM (SELECT jsondata FROM postgres_data ORDER BY id) AS sub")
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Veritabanı sorgusu başarısız: %v", err)
	}
	defer rows.Close()

	var jsonData []byte
	if rows.Next() {
		err := rows.Scan(&jsonData)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Veri okuma hatası: %v", err)
		}
	}

	// JSON verisini structpb.Value'ya dönüştür
	var jsonValue interface{}
	if err := json.Unmarshal(jsonData, &jsonValue); err != nil {
		return nil, status.Errorf(codes.Internal, "JSON ayrıştırma hatası: %v", err)
	}

	value, err := structpb.NewValue(jsonValue)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Veri dönüştürme hatası: %v", err)
	}

	return value, nil
}

// GetConnectedAgents bağlı tüm agent'ların listesini döndürür
func (s *Server) GetConnectedAgents() map[string]*pb.AgentInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*pb.AgentInfo)
	for id, agent := range s.agents {
		result[id] = agent.Info
	}

	return result
}

// Veritabanı bağlantısını kontrol et
func (s *Server) checkDatabaseConnection() error {
	err := s.db.Ping()
	if err != nil {
		log.Printf("Veritabanı bağlantı hatası: %v", err)
		return err
	}
	return nil
}

// SendQuery, belirli bir agent'a sorgu gönderir ve cevabı bekler
func (s *Server) SendQuery(ctx context.Context, agentID, queryID, command string) (*pb.QueryResult, error) {
	s.mu.RLock()
	agentConn, ok := s.agents[agentID]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("agent bulunamadı: %s", agentID)
	}

	// Sorgu cevabı için bir kanal oluştur
	resultChan := make(chan *pb.QueryResult, 1)

	// Haritaya ekle
	s.queryMu.Lock()
	s.queryResult[queryID] = &QueryResponse{
		ResultChan: resultChan,
	}
	s.queryMu.Unlock()

	// Temizlik işlemi için defer
	defer func() {
		s.queryMu.Lock()
		delete(s.queryResult, queryID)
		s.queryMu.Unlock()
		close(resultChan)
	}()

	// Sorguyu agent'a gönder
	err := agentConn.Stream.Send(&pb.ServerMessage{
		Payload: &pb.ServerMessage_Query{
			Query: &pb.Query{
				QueryId: queryID,
				Command: command,
			},
		},
	})

	if err != nil {
		return nil, err
	}

	// Cevabı bekle (timeout ile)
	select {
	case result := <-resultChan:
		return result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(10 * time.Second): // 10 saniye timeout
		return nil, fmt.Errorf("sorgu zaman aşımına uğradı")
	}
}
