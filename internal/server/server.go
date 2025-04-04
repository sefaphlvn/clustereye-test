package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/sefaphlvn/clustereye-test/internal/database"
	pb "github.com/sefaphlvn/clustereye-test/pkg/agent"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/anypb"
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

// Connect, agent'ların bağlanması için stream açar
func (s *Server) Connect(stream pb.AgentService_ConnectServer) error {
	var currentAgentID string
	var companyID int

	log.Println("Yeni agent bağlantı isteği alındı")

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
			log.Printf("Agent bilgileri alındı - ID: %s, Hostname: %s", agentInfo.AgentId, agentInfo.Hostname)

			// Agent anahtarını doğrula
			company, err := s.companyRepo.ValidateAgentKey(context.Background(), agentInfo.Key)
			if err != nil {
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
				return err
			}

			// Agent'ı bağlantı listesine ekle
			s.mu.Lock()
			s.agents[currentAgentID] = &AgentConnection{
				Stream: stream,
				Info:   agentInfo,
			}
			s.mu.Unlock()
			log.Printf("Agent bağlantı listesine eklendi - ID: %s, Toplam bağlantı: %d", currentAgentID, len(s.agents))

			// Başarılı kayıt mesajı gönder
			stream.Send(&pb.ServerMessage{
				Payload: &pb.ServerMessage_Registration{
					Registration: &pb.RegistrationResult{
						Status:  "success",
						Message: "Agent başarıyla kaydedildi",
					},
				},
			})

			log.Printf("Agent başarıyla kaydedildi ve bağlandı: %s (Firma: %s)", currentAgentID, company.CompanyName)

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
	log.Println("Register metodu çağrıldı")
	agentInfo := req.AgentInfo

	// Agent anahtarını doğrula
	company, err := s.companyRepo.ValidateAgentKey(ctx, agentInfo.Key)
	if err != nil {
		log.Printf("Agent kimlik doğrulama hatası: %v", err)
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
		log.Printf("Agent kaydedilemedi: %v", err)
		return &pb.RegisterResponse{
			Registration: &pb.RegistrationResult{
				Status:  "error",
				Message: "Agent kaydedilemedi",
			},
		}, nil
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
		ctx,
		agentInfo.Hostname,
		agentInfo.Platform,     // Platform alanını cluster adı olarak kullanıyoruz
		agentInfo.PostgresUser, // Agent'dan gelen kullanıcı adı
		agentInfo.PostgresPass, // Agent'dan gelen şifre
	)

	if err != nil {
		log.Printf("PostgreSQL bağlantı bilgileri kaydedilemedi: %v", err)
	} else {
		log.Printf("PostgreSQL bağlantı bilgileri kaydedildi: %s", agentInfo.Hostname)
	}

	log.Printf("Yeni Agent bağlandı ve kaydedildi: %+v (Firma: %s)", agentInfo, company.CompanyName)

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
	log.Println("SendPostgresInfo metodu çağrıldı")

	// Gelen PostgreSQL bilgilerini logla
	pgInfo := req.PostgresInfo
	log.Printf("PostgreSQL bilgileri alındı: %+v", pgInfo)

	// Daha detaylı loglama
	log.Printf("Cluster: %s, IP: %s, Hostname: %s", pgInfo.ClusterName, pgInfo.Ip, pgInfo.Hostname)
	log.Printf("Node Durumu: %s, PG Sürümü: %s, Konum: %s", pgInfo.NodeStatus, pgInfo.PgVersion, pgInfo.Location)
	log.Printf("PGBouncer Durumu: %s, PG Servis Durumu: %s", pgInfo.PgBouncerStatus, pgInfo.PgServiceStatus)
	log.Printf("Replikasyon Gecikmesi: %d saniye, Boş Disk: %s, FD Yüzdesi: %d%%",
		pgInfo.ReplicationLagSec, pgInfo.FreeDisk, pgInfo.FdPercent)

	// Veritabanına kaydetme işlemi
	// Bu kısmı ihtiyacınıza göre geliştirebilirsiniz
	err := s.savePostgresInfoToDatabase(ctx, pgInfo)
	if err != nil {
		log.Printf("PostgreSQL bilgileri veritabanına kaydedilemedi: %v", err)
		return &pb.PostgresInfoResponse{
			Status: "error",
		}, nil
	}

	log.Printf("PostgreSQL bilgileri başarıyla işlendi ve kaydedildi")

	return &pb.PostgresInfoResponse{
		Status: "success",
	}, nil
}

// PostgreSQL bilgilerini veritabanına kaydetmek için yardımcı fonksiyon
func (s *Server) savePostgresInfoToDatabase(ctx context.Context, pgInfo *pb.PostgresInfo) error {
	// Önce mevcut kaydı kontrol et
	var existingData []byte
	var id int

	checkQuery := `
		SELECT id, jsondata FROM public.postgres_data 
		WHERE clustername = $1 
		ORDER BY id DESC LIMIT 1
	`

	err := s.db.QueryRowContext(ctx, checkQuery, pgInfo.ClusterName).Scan(&id, &existingData)

	// Yeni node verisi
	pgData := map[string]interface{}{
		"ClusterName":       pgInfo.ClusterName,
		"Location":          pgInfo.Location,
		"FDPercent":         pgInfo.FdPercent,
		"FreeDisk":          pgInfo.FreeDisk,
		"Hostname":          pgInfo.Hostname,
		"IP":                pgInfo.Ip,
		"NodeStatus":        pgInfo.NodeStatus,
		"PGBouncerStatus":   pgInfo.PgBouncerStatus,
		"PGServiceStatus":   pgInfo.PgServiceStatus,
		"PGVersion":         pgInfo.PgVersion,
		"ReplicationLagSec": pgInfo.ReplicationLagSec,
	}

	var jsonData []byte

	if err == nil {
		// Mevcut kayıt var, güncelle
		var existingJSON map[string][]interface{}
		if err := json.Unmarshal(existingData, &existingJSON); err != nil {
			log.Printf("Mevcut JSON ayrıştırma hatası: %v", err)
			return err
		}

		// Cluster array'ini al
		clusterData, ok := existingJSON[pgInfo.ClusterName]
		if !ok {
			// Eğer cluster verisi yoksa yeni oluştur
			clusterData = []interface{}{}
		}

		// Node'u bul ve güncelle
		nodeFound := false
		for i, node := range clusterData {
			nodeMap, ok := node.(map[string]interface{})
			if !ok {
				continue
			}

			// Hostname ve IP ile node eşleşmesi kontrol et
			if nodeMap["Hostname"] == pgInfo.Hostname && nodeMap["IP"] == pgInfo.Ip {
				// Sadece değişen alanları güncelle
				nodeFound = true

				// Mevcut değerleri koru, sadece değişenleri güncelle
				for key, newValue := range pgData {
					if currentValue, exists := nodeMap[key]; !exists || currentValue != newValue {
						nodeMap[key] = newValue
					}
				}

				clusterData[i] = nodeMap
				break
			}
		}

		// Eğer node bulunamadıysa yeni ekle
		if !nodeFound {
			clusterData = append(clusterData, pgData)
			log.Printf("Yeni node eklendi: %s", pgInfo.Hostname)
		}

		existingJSON[pgInfo.ClusterName] = clusterData

		// JSON'ı güncelle
		jsonData, err = json.Marshal(existingJSON)
		if err != nil {
			log.Printf("JSON dönüştürme hatası: %v", err)
			return err
		}

		// Veritabanını güncelle
		updateQuery := `
			UPDATE public.postgres_data 
			SET jsondata = $1 
			WHERE id = $2
		`

		_, err = s.db.ExecContext(ctx, updateQuery, jsonData, id)
		if err != nil {
			log.Printf("Veritabanı güncelleme hatası: %v", err)
			return err
		}

		log.Printf("PostgreSQL node bilgileri başarıyla güncellendi")
	} else {
		// İlk kayıt oluştur
		outerJSON := map[string][]interface{}{
			pgInfo.ClusterName: {pgData},
		}

		jsonData, err = json.Marshal(outerJSON)
		if err != nil {
			log.Printf("JSON dönüştürme hatası: %v", err)
			return err
		}

		insertQuery := `
			INSERT INTO public.postgres_data (
				jsondata, clustername
			) VALUES ($1, $2)
		`

		_, err = s.db.ExecContext(ctx, insertQuery, jsonData, pgInfo.ClusterName)
		if err != nil {
			log.Printf("Veritabanı ekleme hatası: %v", err)
			return err
		}

		log.Printf("PostgreSQL node bilgileri başarıyla veritabanına kaydedildi")
	}

	log.Printf("Kaydedilen/Güncellenen JSON: %s", string(jsonData))
	return nil
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

// GetConnectedAgents, aktif gRPC bağlantılarındaki agent'ları döndürür
func (s *Server) GetConnectedAgents() []map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	log.Printf("Aktif gRPC bağlantıları: %d", len(s.agents))

	// Istanbul zaman dilimini al
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		log.Printf("Zaman dilimi yüklenemedi: %v", err)
		loc = time.UTC
	}

	agents := make([]map[string]interface{}, 0)
	for id, conn := range s.agents {
		if conn == nil || conn.Info == nil {
			log.Printf("Geçersiz agent bağlantısı: %s", id)
			continue
		}

		log.Printf("Agent bulundu - ID: %s, Hostname: %s", id, conn.Info.Hostname)
		agent := map[string]interface{}{
			"id":         id,
			"hostname":   conn.Info.Hostname,
			"ip":         conn.Info.Ip,
			"status":     "connected",
			"last_seen":  time.Now().In(loc).Format("2006-01-02T15:04:05-07:00"),
			"connection": "grpc",
		}
		agents = append(agents, agent)
	}
	return agents
}

// GetAgentStatusFromDB, veritabanından agent durumlarını alır
func (s *Server) GetAgentStatusFromDB(ctx context.Context) ([]map[string]interface{}, error) {
	query := `
		SELECT hostname, last_seen 
		FROM agents 
		WHERE last_seen > NOW() - INTERVAL '1 minute'
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("veritabanı sorgusu başarısız: %v", err)
	}
	defer rows.Close()

	agents := make([]map[string]interface{}, 0)
	for rows.Next() {
		var hostname string
		var lastSeen time.Time
		if err := rows.Scan(&hostname, &lastSeen); err != nil {
			return nil, fmt.Errorf("satır okuma hatası: %v", err)
		}

		agent := map[string]interface{}{
			"hostname":   hostname,
			"last_seen":  lastSeen.Format(time.RFC3339),
			"status":     "active",
			"connection": "db",
		}
		agents = append(agents, agent)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("satır okuma hatası: %v", err)
	}

	return agents, nil
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
	defer s.mu.RUnlock()

	// Agent ID'sini kontrol et ve gerekirse düzelt
	if !strings.HasPrefix(agentID, "agent_") {
		agentID = "agent_" + agentID
	}

	agentConn, ok := s.agents[agentID]
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
		// Protobuf sonucunu JSON formatına dönüştür
		if result.Result != nil {
			log.Printf("Received result type: %s", result.Result.TypeUrl)

			// Protobuf struct'ı parse et
			var structValue structpb.Struct
			if err := result.Result.UnmarshalTo(&structValue); err != nil {
				log.Printf("Error unmarshaling to struct: %v", err)
				return result, nil
			}

			// Struct'ı map'e dönüştür
			resultMap := structValue.AsMap()
			log.Printf("Result map: %+v", resultMap)

			// Map'i JSON'a dönüştür
			jsonBytes, err := json.Marshal(resultMap)
			if err != nil {
				log.Printf("Error marshaling map to JSON: %v", err)
				return result, nil
			}

			// Sonucu güncelle
			result.Result = &anypb.Any{
				TypeUrl: "type.googleapis.com/google.protobuf.Value",
				Value:   jsonBytes,
			}
		}
		return result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(10 * time.Second): // 10 saniye timeout
		return nil, fmt.Errorf("sorgu zaman aşımına uğradı")
	}
}
