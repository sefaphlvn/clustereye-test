package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sefaphlvn/clustereye-test/internal/database"
	pb "github.com/sefaphlvn/clustereye-test/pkg/agent"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
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
	// Son ping zamanlarını tutmak için map
	lastPingMu   sync.RWMutex
	lastPingTime map[string]time.Time
}

// NewServer, yeni bir sunucu nesnesi oluşturur
func NewServer(db *sql.DB) *Server {
	return &Server{
		agents:       make(map[string]*AgentConnection),
		queryResult:  make(map[string]*QueryResponse),
		db:           db,
		companyRepo:  database.NewCompanyRepository(db),
		lastPingTime: make(map[string]time.Time),
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
			log.Printf("Agent %s sorguya cevap verdi.", currentAgentID)

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

		// gRPC stream'in durumunu kontrol et
		status := "disconnected"
		if conn.Stream != nil {
			shouldPing := false

			// Son ping zamanını kontrol et
			s.lastPingMu.RLock()
			lastPing, exists := s.lastPingTime[id]
			s.lastPingMu.RUnlock()

			if !exists || time.Since(lastPing) > 30*time.Second {
				shouldPing = true
			} else {
				// Son 30 saniye içinde başarılı ping varsa, bağlı kabul et
				status = "connected"
			}

			if shouldPing {
				// Ping mesajı gönder
				pingMsg := &pb.ServerMessage{
					Payload: &pb.ServerMessage_Query{
						Query: &pb.Query{
							QueryId: fmt.Sprintf("ping_%d", time.Now().UnixNano()),
							Command: "ping",
						},
					},
				}

				// Ping'i sadece gRPC stream üzerinden gönder
				err := conn.Stream.Send(pingMsg)
				if err == nil {
					status = "connected"
					// Başarılı ping zamanını kaydet
					s.lastPingMu.Lock()
					s.lastPingTime[id] = time.Now()
					s.lastPingMu.Unlock()
				} else {
					log.Printf("Agent %s ping hatası: %v", id, err)
					// Stream'i kapat ve agent'ı sil
					delete(s.agents, id)
					// Son ping zamanını da sil
					s.lastPingMu.Lock()
					delete(s.lastPingTime, id)
					s.lastPingMu.Unlock()
					continue
				}
			}
		}

		log.Printf("Agent bulundu - ID: %s, Hostname: %s, Status: %s", id, conn.Info.Hostname, status)
		agent := map[string]interface{}{
			"id":         id,
			"hostname":   conn.Info.Hostname,
			"ip":         conn.Info.Ip,
			"status":     status,
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

			// Protobuf struct'ı parse et
			var structValue structpb.Struct
			if err := result.Result.UnmarshalTo(&structValue); err != nil {
				log.Printf("Error unmarshaling to struct: %v", err)
				return result, nil
			}

			// Struct'ı map'e dönüştür
			resultMap := structValue.AsMap()

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

// SendSystemMetrics, agent'dan sistem metriklerini alır
func (s *Server) SendSystemMetrics(ctx context.Context, req *pb.SystemMetricsRequest) (*pb.SystemMetricsResponse, error) {
	log.Printf("[DEBUG] SendSystemMetrics başladı: agent_id=%s", req.AgentId)

	s.mu.RLock()
	defer s.mu.RUnlock()

	agentID := req.AgentId
	if !strings.HasPrefix(agentID, "agent_") {
		agentID = "agent_" + agentID
		log.Printf("[DEBUG] Agent ID düzeltildi: %s", agentID)
	}

	agentConn, ok := s.agents[agentID]
	if !ok {
		log.Printf("[ERROR] Agent bulunamadı: %s", agentID)
		return nil, fmt.Errorf("agent bulunamadı: %s", agentID)
	}

	log.Printf("[DEBUG] Agent bağlantısı bulundu: %s", agentID)

	// gRPC stream'in durumunu kontrol et
	if agentConn.Stream == nil {
		log.Printf("[ERROR] gRPC stream nil: agent_id=%s", agentID)
		return nil, fmt.Errorf("gRPC stream bağlantısı kopmuş")
	}

	// Metrik isteğini agent'a gönder
	log.Printf("[DEBUG] Metrik isteği gönderiliyor: agent_id=%s", agentID)
	err := agentConn.Stream.Send(&pb.ServerMessage{
		Payload: &pb.ServerMessage_MetricsRequest{
			MetricsRequest: &pb.SystemMetricsRequest{
				AgentId: agentID,
			},
		},
	})

	if err != nil {
		log.Printf("[ERROR] Metrik isteği gönderilemedi: agent_id=%s, error=%v", agentID, err)
		return nil, err
	}
	log.Printf("[DEBUG] Metrik isteği başarıyla gönderildi: agent_id=%s", agentID)

	// Agent yanıtını bekle
	msg, err := agentConn.Stream.Recv()
	if err != nil {
		log.Printf("[ERROR] Agent yanıtı alınamadı: agent_id=%s, error=%v", agentID, err)
		return &pb.SystemMetricsResponse{
			Status: "error",
		}, err
	}

	log.Printf("[DEBUG] Agent yanıtı alındı: agent_id=%s, payload_type=%T", agentID, msg.Payload)
	if metrics, ok := msg.Payload.(*pb.AgentMessage_SystemMetrics); ok {
		log.Printf("[DEBUG] SystemMetrics yanıtı alındı: agent_id=%s", agentID)
		return &pb.SystemMetricsResponse{
			Status:  "success",
			Metrics: metrics.SystemMetrics,
		}, nil
	} else {
		log.Printf("[ERROR] Beklenmeyen yanıt tipi: agent_id=%s, payload_type=%T", agentID, msg.Payload)
		return &pb.SystemMetricsResponse{
			Status: "error",
		}, fmt.Errorf("beklenmeyen yanıt tipi: %T", msg.Payload)
	}
}

// GetDB, veritabanı bağlantısını döndürür
func (s *Server) GetDB() *sql.DB {
	return s.db
}

// ReportAlarm, agent'lardan gelen alarm bildirimlerini işler
func (s *Server) ReportAlarm(ctx context.Context, req *pb.ReportAlarmRequest) (*pb.ReportAlarmResponse, error) {
	log.Printf("ReportAlarm metodu çağrıldı, Agent ID: %s, Alarm sayısı: %d", req.AgentId, len(req.Events))

	// Agent ID doğrula
	s.mu.RLock()
	_, agentExists := s.agents[req.AgentId]
	s.mu.RUnlock()

	if !agentExists {
		log.Printf("Bilinmeyen agent'dan alarm bildirimi: %s", req.AgentId)
		// Bilinmeyen agent olsa da işlemeye devam ediyoruz
	}

	// Gelen her alarmı işle
	for _, event := range req.Events {
		log.Printf("Alarm işleniyor - ID: %s, Status: %s, Metric: %s, Value: %s, Severity: %s",
			event.Id, event.Status, event.MetricName, event.MetricValue, event.Severity)

		// Alarm verilerini veritabanına kaydet
		err := s.saveAlarmToDatabase(ctx, event)
		if err != nil {
			log.Printf("Alarm veritabanına kaydedilemedi: %v", err)
			// Devam et, bir alarmın kaydedilememesi diğerlerini etkilememeli
		}

		// Bildirimi gönder (Slack, Email vb.)
		err = s.sendAlarmNotification(ctx, event)
		if err != nil {
			log.Printf("Alarm bildirimi gönderilemedi: %v", err)
			// Devam et, bir bildirimin gönderilememesi diğerlerini etkilememeli
		}
	}

	return &pb.ReportAlarmResponse{
		Status: "success",
	}, nil
}

// saveAlarmToDatabase, alarm olayını veritabanına kaydeder
func (s *Server) saveAlarmToDatabase(ctx context.Context, event *pb.AlarmEvent) error {
	// Veritabanı bağlantısını kontrol et
	if err := s.checkDatabaseConnection(); err != nil {
		return fmt.Errorf("veritabanı bağlantı hatası: %v", err)
	}

	// SQL sorgusu hazırla
	query := `
		INSERT INTO alarms (
			alarm_id, 
			event_id, 
			agent_id, 
			status, 
			metric_name, 
			metric_value, 
			message, 
			severity,
			created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	// Zaman damgasını parse et
	timestamp, err := time.Parse(time.RFC3339, event.Timestamp)
	if err != nil {
		timestamp = time.Now() // Parse edilemezse şu anki zamanı kullan
		log.Printf("Zaman damgası parse edilemedi: %v, şu anki zaman kullanılıyor", err)
	}

	// Veritabanına kaydet
	_, err = s.db.ExecContext(
		ctx,
		query,
		event.AlarmId,
		event.Id,
		event.AgentId,
		event.Status,
		event.MetricName,
		event.MetricValue,
		event.Message,
		event.Severity,
		timestamp,
	)

	if err != nil {
		return fmt.Errorf("alarm veritabanına kaydedilemedi: %v", err)
	}

	log.Printf("Alarm veritabanına kaydedildi - ID: %s", event.Id)
	return nil
}

// sendAlarmNotification, alarm olayını ilgili kanallara bildirir
func (s *Server) sendAlarmNotification(ctx context.Context, event *pb.AlarmEvent) error {
	// Notification ayarlarını veritabanından al
	var slackWebhookURL string
	var slackEnabled bool
	var emailEnabled bool
	var emailServer, emailPort, emailUser, emailPassword, emailFrom string
	var emailRecipientsStr string

	query := `
		SELECT 
			slack_webhook_url,
			slack_enabled,
			email_enabled,
			email_server,
			email_port,
			email_user,
			email_password,
			email_from,
			email_recipients
		FROM notification_settings
		ORDER BY id DESC
		LIMIT 1
	`

	err := s.db.QueryRowContext(ctx, query).Scan(
		&slackWebhookURL,
		&slackEnabled,
		&emailEnabled,
		&emailServer,
		&emailPort,
		&emailUser,
		&emailPassword,
		&emailFrom,
		&emailRecipientsStr,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			log.Println("Notification ayarları bulunamadı, bildirim gönderilemiyor")
			return nil // Ayar yok, hata kabul etmiyoruz
		}
		return fmt.Errorf("notification ayarları alınamadı: %v", err)
	}

	// Slack bildirimi gönder
	if slackEnabled && slackWebhookURL != "" {
		err = s.sendSlackNotification(event, slackWebhookURL)
		if err != nil {
			log.Printf("Slack bildirimi gönderilemedi: %v", err)
		}
	}

	// Email bildirimi gönder
	if emailEnabled && emailServer != "" && emailFrom != "" && emailRecipientsStr != "" {
		// PostgreSQL array formatını parse et
		var emailRecipients []string
		if len(emailRecipientsStr) > 2 { // En az {} olmalı
			// PostgreSQL array formatı: {email1,email2,...}
			trimmedStr := emailRecipientsStr[1 : len(emailRecipientsStr)-1] // Başındaki { ve sonundaki } karakterlerini kaldır
			if trimmedStr != "" {
				emailRecipients = strings.Split(trimmedStr, ",")
			}
		}

		if len(emailRecipients) > 0 {
			err = s.sendEmailNotification(event, emailServer, emailPort, emailUser, emailPassword, emailFrom, emailRecipients)
			if err != nil {
				log.Printf("Email bildirimi gönderilemedi: %v", err)
			}
		}
	}

	return nil
}

// sendSlackNotification, Slack webhook'u aracılığıyla bildirim gönderir
func (s *Server) sendSlackNotification(event *pb.AlarmEvent, webhookURL string) error {
	// Alarm durumuna göre emoji ve renk belirle
	var emoji, color string
	if event.Status == "triggered" {
		if event.Severity == "critical" {
			emoji = ":red_circle:"
			color = "#FF0000" // Kırmızı
		} else if event.Severity == "warning" {
			emoji = ":warning:"
			color = "#FFA500" // Turuncu
		} else {
			emoji = ":information_source:"
			color = "#0000FF" // Mavi
		}
	} else if event.Status == "resolved" {
		emoji = ":white_check_mark:"
		color = "#00FF00" // Yeşil
	} else {
		emoji = ":grey_question:"
		color = "#808080" // Gri
	}

	// Mesaj içeriği
	title := fmt.Sprintf("%s Alarm: %s", strings.ToUpper(event.Status), event.MetricName)

	// JSON mesajı oluştur
	message := map[string]interface{}{
		"attachments": []map[string]interface{}{
			{
				"fallback":    fmt.Sprintf("%s %s: %s - %s", emoji, title, event.MetricValue, event.Message),
				"color":       color,
				"title":       title,
				"title_link":  "http://clustereye.io/alarms", // Alarmlar sayfasına yönlendir
				"text":        event.Message,
				"footer":      fmt.Sprintf("Agent: %s | Alarm ID: %s", event.AgentId, event.AlarmId),
				"footer_icon": "https://clustereye.io/favicon.ico", // Varsa logo URL'si
				"ts":          time.Now().Unix(),
				"fields": []map[string]interface{}{
					{
						"title": "Metrik",
						"value": event.MetricName,
						"short": true,
					},
					{
						"title": "Değer",
						"value": event.MetricValue,
						"short": true,
					},
					{
						"title": "Önem",
						"value": event.Severity,
						"short": true,
					},
					{
						"title": "Durum",
						"value": event.Status,
						"short": true,
					},
				},
			},
		},
	}

	// JSON'a dönüştür
	jsonMessage, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("JSON dönüşüm hatası: %v", err)
	}

	// Slack'e gönder
	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(jsonMessage))
	if err != nil {
		return fmt.Errorf("HTTP POST hatası: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack yanıt kodu başarısız: %d", resp.StatusCode)
	}

	log.Printf("Slack bildirimi başarıyla gönderildi - Alarm ID: %s", event.Id)
	return nil
}

// sendEmailNotification, email aracılığıyla bildirim gönderir
func (s *Server) sendEmailNotification(event *pb.AlarmEvent, server, port, user, password, from string, recipients []string) error {
	// Alarm durumuna göre konu oluştur
	var subject string
	if event.Status == "triggered" {
		if event.Severity == "critical" {
			subject = fmt.Sprintf("[KRITIK ALARM] %s: %s", event.MetricName, event.MetricValue)
		} else if event.Severity == "warning" {
			subject = fmt.Sprintf("[UYARI] %s: %s", event.MetricName, event.MetricValue)
		} else {
			subject = fmt.Sprintf("[BILGI] %s: %s", event.MetricName, event.MetricValue)
		}
	} else if event.Status == "resolved" {
		subject = fmt.Sprintf("[COZULDU] %s: %s", event.MetricName, event.MetricValue)
	} else {
		subject = fmt.Sprintf("[DURUM: %s] %s: %s", event.Status, event.MetricName, event.MetricValue)
	}

	// Mesaj içeriğini oluştur
	htmlBody := fmt.Sprintf(`
	<!DOCTYPE html>
	<html>
	<head>
		<style>
			body { font-family: Arial, sans-serif; line-height: 1.6; }
			.header { background-color: #f5f5f5; padding: 20px; border-bottom: 1px solid #ddd; }
			.content { padding: 20px; }
			.footer { background-color: #f5f5f5; padding: 20px; border-top: 1px solid #ddd; font-size: 12px; }
			.alarm-critical { color: #cc0000; }
			.alarm-warning { color: #ff9900; }
			.alarm-info { color: #0066cc; }
			.alarm-resolved { color: #009900; }
			.details { margin-top: 20px; border-top: 1px solid #ddd; padding-top: 20px; }
			.detail-row { display: flex; margin-bottom: 10px; }
			.detail-label { width: 150px; font-weight: bold; }
		</style>
	</head>
	<body>
		<div class="header">
			<h2>ClusterEye Alarm Bildirimi</h2>
		</div>
		<div class="content">
			<h3 class="alarm-%s">%s</h3>
			<p>%s</p>
			
			<div class="details">
				<div class="detail-row">
					<div class="detail-label">Agent ID:</div>
					<div>%s</div>
				</div>
				<div class="detail-row">
					<div class="detail-label">Metrik:</div>
					<div>%s</div>
				</div>
				<div class="detail-row">
					<div class="detail-label">Değer:</div>
					<div>%s</div>
				</div>
				<div class="detail-row">
					<div class="detail-label">Önem Derecesi:</div>
					<div>%s</div>
				</div>
				<div class="detail-row">
					<div class="detail-label">Durum:</div>
					<div>%s</div>
				</div>
				<div class="detail-row">
					<div class="detail-label">Alarm ID:</div>
					<div>%s</div>
				</div>
				<div class="detail-row">
					<div class="detail-label">Olay ID:</div>
					<div>%s</div>
				</div>
				<div class="detail-row">
					<div class="detail-label">Zaman:</div>
					<div>%s</div>
				</div>
			</div>
		</div>
		<div class="footer">
			<p>Bu bildirim <a href="https://clustereye.io">ClusterEye</a> tarafından otomatik olarak gönderilmiştir.</p>
		</div>
	</body>
	</html>
	`,
		event.Severity, // CSS sınıfı için
		subject,        // Başlık
		event.Message,  // Mesaj
		event.AgentId,
		event.MetricName,
		event.MetricValue,
		event.Severity,
		event.Status,
		event.AlarmId,
		event.Id,
		event.Timestamp,
	)

	// Basit metin içeriği
	textBody := fmt.Sprintf("ClusterEye Alarm Bildirimi\n\n%s\n\n%s\n\nAgent: %s\nMetrik: %s\nDeğer: %s\nÖnem: %s\nDurum: %s\nAlarm ID: %s\nOlay ID: %s\nZaman: %s",
		subject,
		event.Message,
		event.AgentId,
		event.MetricName,
		event.MetricValue,
		event.Severity,
		event.Status,
		event.AlarmId,
		event.Id,
		event.Timestamp,
	)

	log.Printf("Email bildirimi gönderiliyor - Konu: %s", subject)
	log.Printf("Email alıcıları: %v", recipients)

	// Email gönderme işlemi burada gerçekleştirilecek
	// Bu kısım şimdilik log kaydı yapmaktadır
	// Gerçek SMTP entegrasyonu için aşağıdaki kodu açabilirsiniz:

	/*
		// SMTP sunucusuna bağlan
		smtpAddr := fmt.Sprintf("%s:%s", server, port)

		// SMTP kimlik doğrulama
		auth := smtp.PlainAuth("", user, password, server)

		// Email içeriği
		mime := "MIME-version: 1.0;\nContent-Type: text/html; charset=\"UTF-8\";\n\n"
		msg := []byte("To: " + strings.Join(recipients, ",") + "\r\n" +
			"From: " + from + "\r\n" +
			"Subject: " + subject + "\r\n" +
			mime + "\r\n" +
			htmlBody + "\r\n")

		// Email gönder
		err := smtp.SendMail(smtpAddr, auth, from, recipients, msg)
		if err != nil {
			return fmt.Errorf("email gönderme hatası: %v", err)
		}
	*/

	// Temizlik için değişkenleri kullanıldı olarak işaretle
	_ = htmlBody
	_ = textBody

	// Başarılı bir şekilde gönderildi
	log.Printf("Email bildirimi başarıyla gönderildi (simüle edildi) - Alarm ID: %s", event.Id)
	return nil
}

// SendMongoInfo, agent'dan gelen MongoDB bilgilerini işler
func (s *Server) SendMongoInfo(ctx context.Context, req *pb.MongoInfoRequest) (*pb.MongoInfoResponse, error) {
	log.Println("SendMongoInfo metodu çağrıldı")

	// Gelen MongoDB bilgilerini logla
	mongoInfo := req.MongoInfo
	log.Printf("MongoDB bilgileri alındı: %+v", mongoInfo)

	// Daha detaylı loglama
	log.Printf("Cluster: %s, IP: %s, Hostname: %s", mongoInfo.ClusterName, mongoInfo.Ip, mongoInfo.Hostname)
	log.Printf("Node Durumu: %s, Mongo Sürümü: %s, Konum: %s", mongoInfo.NodeStatus, mongoInfo.MongoVersion, mongoInfo.Location)
	log.Printf("Mongo Servis Durumu: %s, Replica Set: %s", mongoInfo.MongoStatus, mongoInfo.ReplicaSetName)
	log.Printf("Replikasyon Gecikmesi: %d saniye, Boş Disk: %s, FD Yüzdesi: %d%%",
		mongoInfo.ReplicationLagSec, mongoInfo.FreeDisk, mongoInfo.FdPercent)

	// Veritabanına kaydetme işlemi
	err := s.saveMongoInfoToDatabase(ctx, mongoInfo)
	if err != nil {
		log.Printf("MongoDB bilgileri veritabanına kaydedilemedi: %v", err)
		return &pb.MongoInfoResponse{
			Status: "error",
		}, nil
	}

	log.Printf("MongoDB bilgileri başarıyla işlendi ve kaydedildi")

	return &pb.MongoInfoResponse{
		Status: "success",
	}, nil
}

// MongoDB bilgilerini veritabanına kaydetmek için yardımcı fonksiyon
func (s *Server) saveMongoInfoToDatabase(ctx context.Context, mongoInfo *pb.MongoInfo) error {
	// Önce mevcut kaydı kontrol et
	var existingData []byte
	var id int

	checkQuery := `
		SELECT id, jsondata FROM public.mongo_data 
		WHERE clustername = $1 
		ORDER BY id DESC LIMIT 1
	`

	err := s.db.QueryRowContext(ctx, checkQuery, mongoInfo.ClusterName).Scan(&id, &existingData)

	// Yeni node verisi
	mongoData := map[string]interface{}{
		"ClusterName":       mongoInfo.ClusterName,
		"Location":          mongoInfo.Location,
		"FDPercent":         mongoInfo.FdPercent,
		"FreeDisk":          mongoInfo.FreeDisk,
		"Hostname":          mongoInfo.Hostname,
		"IP":                mongoInfo.Ip,
		"NodeStatus":        mongoInfo.NodeStatus,
		"MongoStatus":       mongoInfo.MongoStatus,
		"MongoVersion":      mongoInfo.MongoVersion,
		"ReplicaSetName":    mongoInfo.ReplicaSetName,
		"ReplicationLagSec": mongoInfo.ReplicationLagSec,
		"Port":              mongoInfo.Port,
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
		clusterData, ok := existingJSON[mongoInfo.ClusterName]
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
			if nodeMap["Hostname"] == mongoInfo.Hostname && nodeMap["IP"] == mongoInfo.Ip {
				// Sadece değişen alanları güncelle
				nodeFound = true

				// Mevcut değerleri koru, sadece değişenleri güncelle
				for key, newValue := range mongoData {
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
			clusterData = append(clusterData, mongoData)
			log.Printf("Yeni MongoDB node eklendi: %s", mongoInfo.Hostname)
		}

		existingJSON[mongoInfo.ClusterName] = clusterData

		// JSON'ı güncelle
		jsonData, err = json.Marshal(existingJSON)
		if err != nil {
			log.Printf("JSON dönüştürme hatası: %v", err)
			return err
		}

		// Veritabanını güncelle
		updateQuery := `
			UPDATE public.mongo_data 
			SET jsondata = $1, updated_at = CURRENT_TIMESTAMP
			WHERE id = $2
		`

		_, err = s.db.ExecContext(ctx, updateQuery, jsonData, id)
		if err != nil {
			log.Printf("Veritabanı güncelleme hatası: %v", err)
			return err
		}

		log.Printf("MongoDB node bilgileri başarıyla güncellendi")
	} else {
		// İlk kayıt oluştur
		outerJSON := map[string][]interface{}{
			mongoInfo.ClusterName: {mongoData},
		}

		jsonData, err = json.Marshal(outerJSON)
		if err != nil {
			log.Printf("JSON dönüştürme hatası: %v", err)
			return err
		}

		insertQuery := `
			INSERT INTO public.mongo_data (
				jsondata, clustername, created_at, updated_at
			) VALUES ($1, $2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`

		_, err = s.db.ExecContext(ctx, insertQuery, jsonData, mongoInfo.ClusterName)
		if err != nil {
			log.Printf("Veritabanı ekleme hatası: %v", err)
			return err
		}

		log.Printf("MongoDB node bilgileri başarıyla veritabanına kaydedildi")
	}

	return nil
}

// GetStatusMongo, MongoDB veritabanından durum bilgilerini çeker
func (s *Server) GetStatusMongo(ctx context.Context, _ *structpb.Struct) (*structpb.Value, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT json_agg(sub.jsondata) FROM (SELECT jsondata FROM mongo_data ORDER BY id) AS sub")
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

// ListMongoLogs, belirtilen agent'tan MongoDB log dosyalarını listeler
func (s *Server) ListMongoLogs(ctx context.Context, req *pb.MongoLogListRequest) (*pb.MongoLogListResponse, error) {
	log.Printf("ListMongoLogs çağrıldı")

	// Agent ID'yi önce metadata'dan almayı dene
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		agentIDValues := md.Get("agent-id")
		if len(agentIDValues) > 0 {
			log.Printf("Metadata'dan agent ID alındı: %s", agentIDValues[0])
			// Metadata'dan gelen agent ID'yi kullan
			agentID := agentIDValues[0]

			// Agent'a istek gönder ve sonucu al
			response, err := s.sendMongoLogListQuery(ctx, agentID)
			if err != nil {
				log.Printf("MongoDB log dosyaları listelenirken hata: %v", err)

				// Daha açıklayıcı hata mesajları için gRPC status kodlarına dönüştür
				if strings.Contains(err.Error(), "agent bulunamadı") {
					return nil, status.Errorf(codes.NotFound, "Agent bulunamadı veya bağlantı kapalı: %s", agentID)
				} else if err == context.DeadlineExceeded {
					return nil, status.Errorf(codes.DeadlineExceeded, "İstek zaman aşımına uğradı")
				}

				return nil, status.Errorf(codes.Internal, "MongoDB log dosyaları listelenirken bir hata oluştu: %v", err)
			}

			log.Printf("MongoDB log dosyaları başarıyla listelendi - Agent: %s, Dosya sayısı: %d",
				agentID, len(response.LogFiles))
			return response, nil
		}
	}

	// Metadata'dan alınamadıysa, context'ten almayı dene
	agentID := ""
	queryCtx, ok := ctx.Value("agent_id").(string)
	if ok && queryCtx != "" {
		agentID = queryCtx
	}

	if agentID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Agent ID belirtilmedi")
	}

	// Agent'a istek gönder ve sonucu al
	response, err := s.sendMongoLogListQuery(ctx, agentID)
	if err != nil {
		log.Printf("MongoDB log dosyaları listelenirken hata: %v", err)

		// Daha açıklayıcı hata mesajları için gRPC status kodlarına dönüştür
		if strings.Contains(err.Error(), "agent bulunamadı") {
			return nil, status.Errorf(codes.NotFound, "Agent bulunamadı veya bağlantı kapalı: %s", agentID)
		} else if err == context.DeadlineExceeded {
			return nil, status.Errorf(codes.DeadlineExceeded, "İstek zaman aşımına uğradı")
		}

		return nil, status.Errorf(codes.Internal, "MongoDB log dosyaları listelenirken bir hata oluştu: %v", err)
	}

	log.Printf("MongoDB log dosyaları başarıyla listelendi - Agent: %s, Dosya sayısı: %d",
		agentID, len(response.LogFiles))
	return response, nil
}

// AnalyzeMongoLog, belirtilen agent'tan MongoDB log dosyasını analiz etmesini ister
func (s *Server) AnalyzeMongoLog(ctx context.Context, req *pb.MongoLogAnalyzeRequest) (*pb.MongoLogAnalyzeResponse, error) {
	log.Printf("AnalyzeMongoLog çağrıldı, log_file_path: '%s' (boş mu? %t), threshold: %d ms, agent_id param: %s",
		req.LogFilePath, req.LogFilePath == "", req.SlowQueryThresholdMs, req.AgentId)

	// Agent ID'yi önce doğrudan istekten al
	agentID := req.AgentId

	// Boşsa metadata'dan almayı dene
	if agentID == "" {
		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			agentIDValues := md.Get("agent-id")
			if len(agentIDValues) > 0 {
				log.Printf("Metadata'dan agent ID alındı: %s", agentIDValues[0])
				agentID = agentIDValues[0]
			}
		}
	}

	// Hala boşsa context'ten almayı dene
	if agentID == "" {
		queryCtx, ok := ctx.Value("agent_id").(string)
		if ok && queryCtx != "" {
			agentID = queryCtx
		}
	}

	log.Printf("Kullanılan agent_id: %s", agentID)

	if agentID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Agent ID belirtilmedi")
	}

	// İstek parametrelerini kontrol et
	if req.LogFilePath == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Log dosya yolu belirtilmedi")
	}

	// Threshold için varsayılan değeri ayarla
	threshold := req.SlowQueryThresholdMs
	if threshold <= 0 {
		threshold = 100 // Varsayılan 100ms
		log.Printf("Threshold değeri 0 veya negatif, varsayılan değer kullanılıyor: %d ms", threshold)
	}

	// Agent'a istek gönder ve sonucu al
	response, err := s.sendMongoLogAnalyzeQuery(ctx, agentID, req.LogFilePath, threshold)
	if err != nil {
		log.Printf("MongoDB log analizi için hata: %v", err)

		// Daha açıklayıcı hata mesajları için gRPC status kodlarına dönüştür
		if strings.Contains(err.Error(), "agent bulunamadı") {
			return nil, status.Errorf(codes.NotFound, "Agent bulunamadı veya bağlantı kapalı: %s", agentID)
		} else if err == context.DeadlineExceeded {
			return nil, status.Errorf(codes.DeadlineExceeded, "İstek zaman aşımına uğradı")
		}

		return nil, status.Errorf(codes.Internal, "MongoDB log analizi için bir hata oluştu: %v", err)
	}

	log.Printf("MongoDB log analizi başarıyla tamamlandı - Agent: %s, Log girişi sayısı: %d",
		agentID, len(response.LogEntries))
	return response, nil
}

// sendMongoLogListQuery, agent'a MongoDB log dosyalarını listelemesi için sorgu gönderir
func (s *Server) sendMongoLogListQuery(ctx context.Context, agentID string) (*pb.MongoLogListResponse, error) {
	// Agent'ın bağlı olup olmadığını kontrol et
	s.mu.RLock()
	agent, agentExists := s.agents[agentID]
	s.mu.RUnlock()

	if !agentExists || agent == nil {
		return nil, fmt.Errorf("agent bulunamadı veya bağlantı kapalı: %s", agentID)
	}

	// MongoDB log dosyalarını listeleyen bir komut oluştur
	command := "list_mongo_logs"
	queryID := fmt.Sprintf("mongo_log_list_%d", time.Now().UnixNano())

	log.Printf("MongoDB log listesi için komut: %s", command)

	// Sonuç kanalı oluştur
	resultChan := make(chan *pb.QueryResult, 1)
	s.queryMu.Lock()
	s.queryResult[queryID] = &QueryResponse{
		ResultChan: resultChan,
	}
	s.queryMu.Unlock()

	// Context'in iptal durumunda kaynakları temizle
	defer func() {
		s.queryMu.Lock()
		delete(s.queryResult, queryID)
		s.queryMu.Unlock()
		close(resultChan)
	}()

	// Sorguyu gönder
	if err := agent.Stream.Send(&pb.ServerMessage{
		Payload: &pb.ServerMessage_Query{
			Query: &pb.Query{
				QueryId: queryID,
				Command: command,
			},
		},
	}); err != nil {
		return nil, fmt.Errorf("sorgu gönderilemedi: %v", err)
	}

	log.Printf("MongoDB log dosyaları için sorgu gönderildi - Agent: %s, QueryID: %s",
		agentID, queryID)

	// Cevabı bekle
	select {
	case result := <-resultChan:
		// Sonuç geldi
		if result == nil {
			return nil, fmt.Errorf("null sorgu sonucu alındı")
		}

		log.Printf("Agent'tan yanıt alındı - QueryID: %s, TypeUrl: %s", queryID, result.Result.TypeUrl)

		// Önce struct olarak ayrıştırmayı dene (Agent'ın gönderdiği tipte)
		var resultStruct structpb.Struct
		if err := result.Result.UnmarshalTo(&resultStruct); err != nil {
			log.Printf("Struct ayrıştırma hatası: %v", err)

			// Struct ayrıştırma başarısız olursa, MongoLogListResponse olarak dene
			var logListResponse pb.MongoLogListResponse
			if err := result.Result.UnmarshalTo(&logListResponse); err != nil {
				log.Printf("MongoLogListResponse ayrıştırma hatası: %v", err)
				return nil, fmt.Errorf("sonuç ayrıştırma hatası: %v", err)
			}

			log.Printf("Doğrudan MongoLogListResponse'a başarıyla ayrıştırıldı - Dosya sayısı: %d", len(logListResponse.LogFiles))
			return &logListResponse, nil
		}

		// Sonucun içeriğini logla
		structBytes, _ := json.Marshal(resultStruct.AsMap())
		log.Printf("Struct içeriği: %s", string(structBytes))

		// Struct'tan MongoLogListResponse oluştur
		logFiles := make([]*pb.MongoLogFile, 0)
		filesValue, ok := resultStruct.Fields["log_files"]
		if ok && filesValue != nil && filesValue.GetListValue() != nil {
			for _, fileValue := range filesValue.GetListValue().Values {
				if fileValue.GetStructValue() != nil {
					fileStruct := fileValue.GetStructValue()

					// Dosya değerlerini al
					nameValue := fileStruct.Fields["name"].GetStringValue()
					pathValue := fileStruct.Fields["path"].GetStringValue()
					sizeValue := int64(fileStruct.Fields["size"].GetNumberValue())
					lastModifiedValue := int64(fileStruct.Fields["last_modified"].GetNumberValue())

					// MongoLogFile oluştur
					logFile := &pb.MongoLogFile{
						Name:         nameValue,
						Path:         pathValue,
						Size:         sizeValue,
						LastModified: lastModifiedValue,
					}

					logFiles = append(logFiles, logFile)
				}
			}
		}

		log.Printf("Struct'tan oluşturulan log dosyaları sayısı: %d", len(logFiles))

		return &pb.MongoLogListResponse{
			LogFiles: logFiles,
		}, nil

	case <-ctx.Done():
		// Context iptal edildi veya zaman aşımına uğradı
		return nil, ctx.Err()
	}
}

// sendMongoLogAnalyzeQuery, agent'a MongoDB log dosyasını analiz etmesi için sorgu gönderir
func (s *Server) sendMongoLogAnalyzeQuery(ctx context.Context, agentID, logFilePath string, thresholdMs int64) (*pb.MongoLogAnalyzeResponse, error) {
	// Agent'ın bağlı olup olmadığını kontrol et
	s.mu.RLock()
	agent, agentExists := s.agents[agentID]
	s.mu.RUnlock()

	if !agentExists || agent == nil {
		return nil, fmt.Errorf("agent bulunamadı veya bağlantı kapalı: %s", agentID)
	}

	// logFilePath boş mu kontrol et
	if logFilePath == "" {
		return nil, fmt.Errorf("log dosya yolu boş olamaz")
	}

	// MongoDB log analizi için bir komut oluştur
	command := fmt.Sprintf("analyze_mongo_log|%s|%d", logFilePath, thresholdMs)
	queryID := fmt.Sprintf("mongo_log_analyze_%d", time.Now().UnixNano())

	log.Printf("MongoDB log analizi için komut: %s", command)

	// Sonuç kanalı oluştur
	resultChan := make(chan *pb.QueryResult, 1)
	s.queryMu.Lock()
	s.queryResult[queryID] = &QueryResponse{
		ResultChan: resultChan,
	}
	s.queryMu.Unlock()

	// Context'in iptal durumunda kaynakları temizle
	defer func() {
		s.queryMu.Lock()
		delete(s.queryResult, queryID)
		s.queryMu.Unlock()
		close(resultChan)
	}()

	// Sorguyu gönder
	err := agent.Stream.Send(&pb.ServerMessage{
		Payload: &pb.ServerMessage_Query{
			Query: &pb.Query{
				QueryId: queryID,
				Command: command,
			},
		},
	})
	if err != nil {
		log.Printf("MongoDB log analizi sorgusu gönderilemedi - Hata: %v", err)
		return nil, fmt.Errorf("sorgu gönderilemedi: %v", err)
	}

	log.Printf("MongoDB log analizi için sorgu gönderildi - Agent: %s, Path: %s, QueryID: %s",
		agentID, logFilePath, queryID)

	// Cevabı bekle
	select {
	case result := <-resultChan:
		// Sonuç geldi
		if result == nil {
			log.Printf("Null sorgu sonucu alındı - QueryID: %s", queryID)
			return nil, fmt.Errorf("null sorgu sonucu alındı")
		}

		log.Printf("Agent'tan yanıt alındı - QueryID: %s, TypeUrl: %s", queryID, result.Result.TypeUrl)

		// Önce struct olarak ayrıştırmayı dene (Agent'ın gönderdiği tipte)
		var resultStruct structpb.Struct
		if err := result.Result.UnmarshalTo(&resultStruct); err != nil {
			log.Printf("Struct ayrıştırma hatası: %v", err)

			// Struct ayrıştırma başarısız olursa, MongoLogAnalyzeResponse olarak dene
			var analyzeResponse pb.MongoLogAnalyzeResponse
			if err := result.Result.UnmarshalTo(&analyzeResponse); err != nil {
				log.Printf("MongoLogAnalyzeResponse ayrıştırma hatası: %v", err)
				return nil, fmt.Errorf("sonuç ayrıştırma hatası: %v", err)
			}

			log.Printf("Doğrudan MongoLogAnalyzeResponse'a başarıyla ayrıştırıldı - Log girişleri: %d", len(analyzeResponse.LogEntries))
			return &analyzeResponse, nil
		}

		// Sonucun içeriğini logla
		structBytes, _ := json.Marshal(resultStruct.AsMap())
		log.Printf("Struct içeriği: %s", string(structBytes))

		// Struct'tan MongoLogAnalyzeResponse oluştur
		logEntries := make([]*pb.MongoLogEntry, 0)
		entriesValue, ok := resultStruct.Fields["log_entries"]
		if ok && entriesValue != nil && entriesValue.GetListValue() != nil {
			for _, entryValue := range entriesValue.GetListValue().Values {
				if entryValue.GetStructValue() != nil {
					entryStruct := entryValue.GetStructValue()

					// Log giriş değerlerini al
					timestamp := int64(entryStruct.Fields["timestamp"].GetNumberValue())
					severity := entryStruct.Fields["severity"].GetStringValue() // String olarak al
					component := entryStruct.Fields["component"].GetStringValue()
					context := entryStruct.Fields["context"].GetStringValue()
					message := entryStruct.Fields["message"].GetStringValue()
					dbName := entryStruct.Fields["db_name"].GetStringValue()
					durationMillis := int64(entryStruct.Fields["duration_millis"].GetNumberValue())
					command := entryStruct.Fields["command"].GetStringValue()
					planSummary := entryStruct.Fields["plan_summary"].GetStringValue()
					namespace := entryStruct.Fields["namespace"].GetStringValue()

					// MongoLogEntry oluştur
					logEntry := &pb.MongoLogEntry{
						Timestamp:      timestamp,
						Severity:       severity, // String olarak kullan
						Component:      component,
						Context:        context,
						Message:        message,
						DbName:         dbName,
						DurationMillis: durationMillis,
						Command:        command,
						PlanSummary:    planSummary,
						Namespace:      namespace,
					}

					logEntries = append(logEntries, logEntry)
				}
			}
		}

		log.Printf("Struct'tan oluşturulan log girişleri sayısı: %d", len(logEntries))

		return &pb.MongoLogAnalyzeResponse{
			LogEntries: logEntries,
		}, nil

	case <-ctx.Done():
		// Context iptal edildi veya zaman aşımına uğradı
		log.Printf("Context iptal edildi veya zaman aşımına uğradı - QueryID: %s", queryID)
		return nil, ctx.Err()
	}
}

// sendPostgresLogListQuery, agent'a PostgreSQL log dosyalarını listelemesi için sorgu gönderir
func (s *Server) sendPostgresLogListQuery(ctx context.Context, agentID string) (*pb.PostgresLogListResponse, error) {
	// Agent'ın bağlı olup olmadığını kontrol et
	s.mu.RLock()
	agent, agentExists := s.agents[agentID]
	s.mu.RUnlock()

	if !agentExists || agent == nil {
		return nil, fmt.Errorf("agent bulunamadı veya bağlantı kapalı: %s", agentID)
	}

	// PostgreSQL log dosyalarını listeleyen bir komut oluştur
	command := "list_postgres_logs"
	queryID := fmt.Sprintf("postgres_log_list_%d", time.Now().UnixNano())

	log.Printf("PostgreSQL log listesi için komut: %s", command)

	// Sonuç kanalı oluştur
	resultChan := make(chan *pb.QueryResult, 1)
	s.queryMu.Lock()
	s.queryResult[queryID] = &QueryResponse{
		ResultChan: resultChan,
	}
	s.queryMu.Unlock()

	// Context'in iptal durumunda kaynakları temizle
	defer func() {
		s.queryMu.Lock()
		delete(s.queryResult, queryID)
		s.queryMu.Unlock()
		close(resultChan)
	}()

	// Sorguyu gönder
	if err := agent.Stream.Send(&pb.ServerMessage{
		Payload: &pb.ServerMessage_Query{
			Query: &pb.Query{
				QueryId: queryID,
				Command: command,
			},
		},
	}); err != nil {
		return nil, fmt.Errorf("sorgu gönderilemedi: %v", err)
	}

	log.Printf("PostgreSQL log dosyaları için sorgu gönderildi - Agent: %s, QueryID: %s",
		agentID, queryID)

	// Cevabı bekle
	select {
	case result := <-resultChan:
		// Sonuç geldi
		if result == nil {
			return nil, fmt.Errorf("null sorgu sonucu alındı")
		}

		log.Printf("Agent'tan yanıt alındı - QueryID: %s, TypeUrl: %s", queryID, result.Result.TypeUrl)

		// Önce struct olarak ayrıştırmayı dene (Agent'ın gönderdiği tipte)
		var resultStruct structpb.Struct
		if err := result.Result.UnmarshalTo(&resultStruct); err != nil {
			log.Printf("Struct ayrıştırma hatası: %v", err)

			// Struct ayrıştırma başarısız olursa, PostgresLogListResponse olarak dene
			var logListResponse pb.PostgresLogListResponse
			if err := result.Result.UnmarshalTo(&logListResponse); err != nil {
				log.Printf("PostgresLogListResponse ayrıştırma hatası: %v", err)
				return nil, fmt.Errorf("sonuç ayrıştırma hatası: %v", err)
			}

			log.Printf("Doğrudan PostgresLogListResponse'a başarıyla ayrıştırıldı - Dosya sayısı: %d", len(logListResponse.LogFiles))
			return &logListResponse, nil
		}

		// Sonucun içeriğini logla
		structBytes, _ := json.Marshal(resultStruct.AsMap())
		log.Printf("Struct içeriği: %s", string(structBytes))

		// Struct'tan PostgresLogListResponse oluştur
		logFiles := make([]*pb.PostgresLogFile, 0)
		filesValue, ok := resultStruct.Fields["log_files"]
		if ok && filesValue != nil && filesValue.GetListValue() != nil {
			for _, fileValue := range filesValue.GetListValue().Values {
				if fileValue.GetStructValue() != nil {
					fileStruct := fileValue.GetStructValue()

					// Dosya değerlerini al
					nameValue := fileStruct.Fields["name"].GetStringValue()
					pathValue := fileStruct.Fields["path"].GetStringValue()
					sizeValue := int64(fileStruct.Fields["size"].GetNumberValue())
					lastModifiedValue := int64(fileStruct.Fields["last_modified"].GetNumberValue())

					// PostgresLogFile oluştur
					logFile := &pb.PostgresLogFile{
						Name:         nameValue,
						Path:         pathValue,
						Size:         sizeValue,
						LastModified: lastModifiedValue,
					}

					logFiles = append(logFiles, logFile)
				}
			}
		}

		log.Printf("Struct'tan oluşturulan log dosyaları sayısı: %d", len(logFiles))

		return &pb.PostgresLogListResponse{
			LogFiles: logFiles,
		}, nil

	case <-ctx.Done():
		// Context iptal edildi veya zaman aşımına uğradı
		return nil, ctx.Err()
	}
}

// ListPostgresLogs, belirtilen agent'tan PostgreSQL log dosyalarını listeler
func (s *Server) ListPostgresLogs(ctx context.Context, req *pb.PostgresLogListRequest) (*pb.PostgresLogListResponse, error) {
	log.Printf("ListPostgresLogs çağrıldı")

	// Agent ID'yi önce metadata'dan almayı dene
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		agentIDValues := md.Get("agent-id")
		if len(agentIDValues) > 0 {
			log.Printf("Metadata'dan agent ID alındı: %s", agentIDValues[0])
			// Metadata'dan gelen agent ID'yi kullan
			agentID := agentIDValues[0]

			// Agent'a istek gönder ve sonucu al
			response, err := s.sendPostgresLogListQuery(ctx, agentID)
			if err != nil {
				log.Printf("PostgreSQL log dosyaları listelenirken hata: %v", err)

				// Daha açıklayıcı hata mesajları için gRPC status kodlarına dönüştür
				if strings.Contains(err.Error(), "agent bulunamadı") {
					return nil, status.Errorf(codes.NotFound, "Agent bulunamadı veya bağlantı kapalı: %s", agentID)
				} else if err == context.DeadlineExceeded {
					return nil, status.Errorf(codes.DeadlineExceeded, "İstek zaman aşımına uğradı")
				}

				return nil, status.Errorf(codes.Internal, "PostgreSQL log dosyaları listelenirken bir hata oluştu: %v", err)
			}

			log.Printf("PostgreSQL log dosyaları başarıyla listelendi - Agent: %s, Dosya sayısı: %d",
				agentID, len(response.LogFiles))
			return response, nil
		}
	}

	// Metadata'dan alınamadıysa, context'ten almayı dene
	agentID := ""
	queryCtx, ok := ctx.Value("agent_id").(string)
	if ok && queryCtx != "" {
		agentID = queryCtx
	}

	if agentID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Agent ID belirtilmedi")
	}

	// Agent'a istek gönder ve sonucu al
	response, err := s.sendPostgresLogListQuery(ctx, agentID)
	if err != nil {
		log.Printf("PostgreSQL log dosyaları listelenirken hata: %v", err)

		// Daha açıklayıcı hata mesajları için gRPC status kodlarına dönüştür
		if strings.Contains(err.Error(), "agent bulunamadı") {
			return nil, status.Errorf(codes.NotFound, "Agent bulunamadı veya bağlantı kapalı: %s", agentID)
		} else if err == context.DeadlineExceeded {
			return nil, status.Errorf(codes.DeadlineExceeded, "İstek zaman aşımına uğradı")
		}

		return nil, status.Errorf(codes.Internal, "PostgreSQL log dosyaları listelenirken bir hata oluştu: %v", err)
	}

	log.Printf("PostgreSQL log dosyaları başarıyla listelendi - Agent: %s, Dosya sayısı: %d",
		agentID, len(response.LogFiles))
	return response, nil
}

// AnalyzePostgresLog, belirtilen PostgreSQL log dosyasını analiz eder
func (s *Server) AnalyzePostgresLog(ctx context.Context, req *pb.PostgresLogAnalyzeRequest) (*pb.PostgresLogAnalyzeResponse, error) {
	// Agent ID'yi context'ten al
	agentID := req.AgentId
	if agentID == "" {
		return nil, fmt.Errorf("agent_id gerekli")
	}

	// Log dosya yolunu kontrol et
	if req.LogFilePath == "" {
		return nil, fmt.Errorf("log_file_path gerekli")
	}

	// Varsayılan threshold değerini ayarla
	thresholdMs := req.SlowQueryThresholdMs
	if thresholdMs <= 0 {
		thresholdMs = 1000 // Varsayılan 1 saniye
	}

	// PostgreSQL log analizi isteğini gönder
	return s.sendPostgresLogAnalyzeQuery(ctx, agentID, req.LogFilePath, thresholdMs)
}

// sendPostgresLogAnalyzeQuery, agent'a PostgreSQL log dosyasını analiz etmesi için sorgu gönderir
func (s *Server) sendPostgresLogAnalyzeQuery(ctx context.Context, agentID, logFilePath string, thresholdMs int64) (*pb.PostgresLogAnalyzeResponse, error) {
	// Agent'ın bağlı olup olmadığını kontrol et
	s.mu.RLock()
	agent, agentExists := s.agents[agentID]
	s.mu.RUnlock()

	if !agentExists || agent == nil {
		return nil, fmt.Errorf("agent bulunamadı veya bağlantı kapalı: %s", agentID)
	}

	// logFilePath boş mu kontrol et
	if logFilePath == "" {
		return nil, fmt.Errorf("log dosya yolu boş olamaz")
	}

	// PostgreSQL log analizi için bir komut oluştur
	command := fmt.Sprintf("analyze_postgres_log|%s|%d", logFilePath, thresholdMs)
	queryID := fmt.Sprintf("postgres_log_analyze_%d", time.Now().UnixNano())

	log.Printf("PostgreSQL log analizi için komut: %s", command)

	// Sonuç kanalı oluştur
	resultChan := make(chan *pb.QueryResult, 1)
	s.queryMu.Lock()
	s.queryResult[queryID] = &QueryResponse{
		ResultChan: resultChan,
	}
	s.queryMu.Unlock()

	// Context'in iptal durumunda kaynakları temizle
	defer func() {
		s.queryMu.Lock()
		delete(s.queryResult, queryID)
		s.queryMu.Unlock()
		close(resultChan)
	}()

	// Sorguyu gönder
	err := agent.Stream.Send(&pb.ServerMessage{
		Payload: &pb.ServerMessage_Query{
			Query: &pb.Query{
				QueryId: queryID,
				Command: command,
			},
		},
	})
	if err != nil {
		log.Printf("PostgreSQL log analizi sorgusu gönderilemedi - Hata: %v", err)
		return nil, fmt.Errorf("sorgu gönderilemedi: %v", err)
	}

	log.Printf("PostgreSQL log analizi için sorgu gönderildi - Agent: %s, Path: %s, QueryID: %s",
		agentID, logFilePath, queryID)

	// Cevabı bekle
	select {
	case result := <-resultChan:
		// Sonuç geldi
		if result == nil {
			log.Printf("Null sorgu sonucu alındı - QueryID: %s", queryID)
			return nil, fmt.Errorf("null sorgu sonucu alındı")
		}

		log.Printf("Agent'tan yanıt alındı - QueryID: %s, TypeUrl: %s", queryID, result.Result.TypeUrl)

		// Önce struct olarak ayrıştırmayı dene (Agent'ın gönderdiği tipte)
		var resultStruct structpb.Struct
		if err := result.Result.UnmarshalTo(&resultStruct); err != nil {
			log.Printf("Struct ayrıştırma hatası: %v", err)

			// Struct ayrıştırma başarısız olursa, PostgresLogAnalyzeResponse olarak dene
			var analyzeResponse pb.PostgresLogAnalyzeResponse
			if err := result.Result.UnmarshalTo(&analyzeResponse); err != nil {
				log.Printf("PostgresLogAnalyzeResponse ayrıştırma hatası: %v", err)
				return nil, fmt.Errorf("sonuç ayrıştırma hatası: %v", err)
			}

			log.Printf("Doğrudan PostgresLogAnalyzeResponse'a başarıyla ayrıştırıldı - Log girişleri: %d", len(analyzeResponse.LogEntries))
			return &analyzeResponse, nil
		}

		// Sonucun içeriğini logla
		json.Marshal(resultStruct.AsMap())

		// Struct'tan PostgresLogAnalyzeResponse oluştur
		logEntries := make([]*pb.PostgresLogEntry, 0)
		entriesValue, ok := resultStruct.Fields["log_entries"]
		if ok && entriesValue != nil && entriesValue.GetListValue() != nil {
			for _, entryValue := range entriesValue.GetListValue().Values {
				if entryValue.GetStructValue() != nil {
					entryStruct := entryValue.GetStructValue()

					// Log giriş değerlerini al
					timestamp := int64(entryStruct.Fields["timestamp"].GetNumberValue())
					logLevel := entryStruct.Fields["log_level"].GetStringValue()
					userName := entryStruct.Fields["user_name"].GetStringValue()
					database := entryStruct.Fields["database"].GetStringValue()
					processId := entryStruct.Fields["process_id"].GetStringValue()
					connectionFrom := entryStruct.Fields["connection_from"].GetStringValue()
					sessionId := entryStruct.Fields["session_id"].GetStringValue()
					sessionLineNum := entryStruct.Fields["session_line_num"].GetStringValue()
					commandTag := entryStruct.Fields["command_tag"].GetStringValue()
					sessionStartTime := entryStruct.Fields["session_start_time"].GetStringValue()
					virtualTransactionId := entryStruct.Fields["virtual_transaction_id"].GetStringValue()
					transactionId := entryStruct.Fields["transaction_id"].GetStringValue()
					errorSeverity := entryStruct.Fields["error_severity"].GetStringValue()
					sqlStateCode := entryStruct.Fields["sql_state_code"].GetStringValue()
					message := entryStruct.Fields["message"].GetStringValue()
					detail := entryStruct.Fields["detail"].GetStringValue()
					hint := entryStruct.Fields["hint"].GetStringValue()
					internalQuery := entryStruct.Fields["internal_query"].GetStringValue()
					durationMs := int64(entryStruct.Fields["duration_ms"].GetNumberValue())

					// PostgresLogEntry oluştur
					logEntry := &pb.PostgresLogEntry{
						Timestamp:            timestamp,
						LogLevel:             logLevel,
						UserName:             userName,
						Database:             database,
						ProcessId:            processId,
						ConnectionFrom:       connectionFrom,
						SessionId:            sessionId,
						SessionLineNum:       sessionLineNum,
						CommandTag:           commandTag,
						SessionStartTime:     sessionStartTime,
						VirtualTransactionId: virtualTransactionId,
						TransactionId:        transactionId,
						ErrorSeverity:        errorSeverity,
						SqlStateCode:         sqlStateCode,
						Message:              message,
						Detail:               detail,
						Hint:                 hint,
						InternalQuery:        internalQuery,
						DurationMs:           durationMs,
					}

					logEntries = append(logEntries, logEntry)
				}
			}
		}

		log.Printf("Struct'tan oluşturulan log girişleri sayısı: %d", len(logEntries))

		return &pb.PostgresLogAnalyzeResponse{
			LogEntries: logEntries,
		}, nil

	case <-ctx.Done():
		// Context iptal edildi veya zaman aşımına uğradı
		log.Printf("Context iptal edildi veya zaman aşımına uğradı - QueryID: %s", queryID)
		return nil, ctx.Err()
	}
}

// GetAlarms, veritabanından alarm kayıtlarını çeker
func (s *Server) GetAlarms(ctx context.Context, onlyUnacknowledged bool) ([]map[string]interface{}, error) {
	// Veritabanı bağlantısını kontrol et
	if err := s.checkDatabaseConnection(); err != nil {
		return nil, fmt.Errorf("veritabanı bağlantı hatası: %v", err)
	}

	// SQL sorgusu
	query := `
		SELECT 
			alarm_id,
			event_id,
			agent_id,
			status,
			metric_name,
			metric_value,
			message,
			severity,
			created_at,
			acknowledged
		FROM alarms
	`

	// Sadece acknowledge edilmemiş alarmları getir
	if onlyUnacknowledged {
		query += " WHERE acknowledged = false"
	}

	query += " ORDER BY created_at DESC"

	// Sorguyu çalıştır
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("alarm verileri çekilemedi: %v", err)
	}
	defer rows.Close()

	// Sonuçları topla
	alarms := make([]map[string]interface{}, 0)
	for rows.Next() {
		var (
			alarmID      string
			eventID      string
			agentID      string
			status       string
			metricName   string
			metricValue  string
			message      string
			severity     string
			createdAt    time.Time
			acknowledged bool
		)

		// Satırı oku
		err := rows.Scan(
			&alarmID,
			&eventID,
			&agentID,
			&status,
			&metricName,
			&metricValue,
			&message,
			&severity,
			&createdAt,
			&acknowledged,
		)
		if err != nil {
			return nil, fmt.Errorf("satır okuma hatası: %v", err)
		}

		// Her alarmı map olarak oluştur
		alarm := map[string]interface{}{
			"alarm_id":     alarmID,
			"event_id":     eventID,
			"agent_id":     agentID,
			"status":       status,
			"metric_name":  metricName,
			"metric_value": metricValue,
			"message":      message,
			"severity":     severity,
			"created_at":   createdAt.Format(time.RFC3339),
			"acknowledged": acknowledged,
		}

		alarms = append(alarms, alarm)
	}

	// Satır okuma hatası kontrolü
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("satır okuma hatası: %v", err)
	}

	return alarms, nil
}

// GetAlarmsStatus, alarm verilerini döndürür
func (s *Server) GetAlarmsStatus(ctx context.Context, _ *structpb.Struct) (*structpb.Value, error) {
	// Alarmları getir
	alarms, err := s.GetAlarms(ctx, false)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Alarm verileri alınamadı: %v", err)
	}

	// JSON verisini structpb.Value'ya dönüştür
	value, err := structpb.NewValue(alarms)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Veri dönüştürme hatası: %v", err)
	}

	return value, nil
}

// AcknowledgeAlarm, belirtilen event_id'ye sahip alarmı acknowledge eder
func (s *Server) AcknowledgeAlarm(ctx context.Context, eventID string) error {
	// Veritabanı bağlantısını kontrol et
	if err := s.checkDatabaseConnection(); err != nil {
		return fmt.Errorf("veritabanı bağlantı hatası: %v", err)
	}

	// SQL sorgusu
	query := `
		UPDATE alarms 
		SET acknowledged = true 
		WHERE event_id = $1
	`

	result, err := s.db.ExecContext(ctx, query, eventID)
	if err != nil {
		return fmt.Errorf("alarm güncellenemedi: %v", err)
	}

	// Etkilenen satır sayısını kontrol et
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("etkilenen satır sayısı alınamadı: %v", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("belirtilen event_id ile alarm bulunamadı: %s", eventID)
	}

	return nil
}
