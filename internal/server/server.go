package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"reflect"
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
	"google.golang.org/protobuf/types/known/timestamppb"
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
	// Job yönetimi için yeni alanlar
	jobMu sync.RWMutex
	jobs  map[string]*pb.Job
}

// NewServer, yeni bir sunucu nesnesi oluşturur
func NewServer(db *sql.DB) *Server {
	return &Server{
		agents:       make(map[string]*AgentConnection),
		queryResult:  make(map[string]*QueryResponse),
		db:           db,
		companyRepo:  database.NewCompanyRepository(db),
		lastPingTime: make(map[string]time.Time),
		jobs:         make(map[string]*pb.Job),
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
	log.Printf("Config Yolu: %s, Data Yolu: %s", pgInfo.ConfigPath, pgInfo.DataPath)

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
		"TotalVCPU":         pgInfo.TotalVcpu,
		"TotalMemory":       pgInfo.TotalMemory,
		"ConfigPath":        pgInfo.ConfigPath,
		"DataPath":          pgInfo.DataPath,
	}

	var jsonData []byte

	// Hata kontrolünü düzgün yap
	if err == nil {
		// Mevcut kayıt var, güncelle
		log.Printf("PostgreSQL cluster için mevcut kayıt bulundu, güncelleniyor: %s (ID: %d)", pgInfo.ClusterName, id)

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
		nodeChanged := false
		for i, node := range clusterData {
			nodeMap, ok := node.(map[string]interface{})
			if !ok {
				continue
			}

			// Hostname ve IP ile node eşleşmesi kontrol et
			if nodeMap["Hostname"] == pgInfo.Hostname && nodeMap["IP"] == pgInfo.Ip {
				// Sadece değişen alanları güncelle
				nodeFound = true

				// Değişiklikleri takip et
				for key, newValue := range pgData {
					currentValue, exists := nodeMap[key]
					var hasChanged bool

					if !exists {
						// Değer mevcut değil, yeni alan ekleniyor
						hasChanged = true
						log.Printf("PostgreSQL node'da yeni alan eklendi: %s, %s", pgInfo.Hostname, key)
					} else {
						// Mevcut değer ile yeni değeri karşılaştır
						// Numeric değerler için özel karşılaştırma yap
						hasChanged = !compareValues(currentValue, newValue)
						if hasChanged {
							log.Printf("PostgreSQL node'da değişiklik tespit edildi: %s, %s: %v -> %v",
								pgInfo.Hostname, key, currentValue, newValue)
						}
					}

					if hasChanged {
						nodeMap[key] = newValue
						// NodeStatus, PGServiceStatus, FreeDisk gibi önemli alanlar değiştiyse işaretle
						if key == "NodeStatus" || key == "PGServiceStatus" || key == "FreeDisk" || key == "ReplicationLagSec" || key == "PGBouncerStatus" {
							nodeChanged = true
						}
					}
				}

				clusterData[i] = nodeMap
				break
			}
		}

		// Eğer node bulunamadıysa yeni ekle
		if !nodeFound {
			clusterData = append(clusterData, pgData)
			nodeChanged = true
			log.Printf("Yeni PostgreSQL node eklendi: %s", pgInfo.Hostname)
		}

		// Eğer önemli bir değişiklik yoksa veritabanını güncelleme
		if !nodeChanged {
			log.Printf("PostgreSQL node'da önemli bir değişiklik yok, güncelleme yapılmadı: %s", pgInfo.Hostname)
			return nil
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
			SET jsondata = $1, updated_at = CURRENT_TIMESTAMP
			WHERE id = $2
		`

		_, err = s.db.ExecContext(ctx, updateQuery, jsonData, id)
		if err != nil {
			log.Printf("Veritabanı güncelleme hatası: %v", err)
			return err
		}

		log.Printf("PostgreSQL node bilgileri başarıyla güncellendi (önemli değişiklik nedeniyle)")
	} else if err == sql.ErrNoRows {
		// Kayıt bulunamadı, yeni kayıt oluştur
		log.Printf("PostgreSQL cluster için kayıt bulunamadı, yeni kayıt oluşturuluyor: %s", pgInfo.ClusterName)

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
				jsondata, clustername, created_at, updated_at
			) VALUES ($1, $2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			ON CONFLICT (clustername) DO UPDATE SET
				jsondata = EXCLUDED.jsondata,
				updated_at = CURRENT_TIMESTAMP
		`

		_, err = s.db.ExecContext(ctx, insertQuery, jsonData, pgInfo.ClusterName)
		if err != nil {
			log.Printf("Veritabanı ekleme hatası: %v", err)
			return err
		}

		log.Printf("PostgreSQL node bilgileri başarıyla veritabanına kaydedildi (yeni kayıt)")
	} else {
		// Başka bir veritabanı hatası oluştu
		log.Printf("PostgreSQL cluster kayıt kontrolü sırasında hata: %v", err)
		return fmt.Errorf("veritabanı kontrol hatası: %v", err)
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
func (s *Server) SendQuery(ctx context.Context, agentID, queryID, command, database string) (*pb.QueryResult, error) {
	// Detaylı loglama ekleyelim
	log.Printf("[DEBUG] ------ Query Detayları ------")
	log.Printf("[DEBUG] Agent ID: %s", agentID)
	log.Printf("[DEBUG] Query ID: %s", queryID)
	log.Printf("[DEBUG] Database: %s", database)
	log.Printf("[DEBUG] Komut Türü: %s", strings.Split(command, ":")[0])
	log.Printf("[DEBUG] Komut Uzunluğu: %d bytes", len(command))

	// Sorguyu da yazdıralım, ama çok uzunsa kısaltarak
	if len(command) > 1000 {
		log.Printf("[DEBUG] Komut (ilk 1000 karakter): %s...", command[:1000])
	} else {
		log.Printf("[DEBUG] Komut: %s", command)
	}
	log.Printf("[DEBUG] -----------------------------")

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
				QueryId:  queryID,
				Command:  command,
				Database: database,
			},
		},
	})

	if err != nil {
		log.Printf("[ERROR] Sorgu gönderimi başarısız: %v", err)
		return nil, err
	}

	log.Printf("[DEBUG] Sorgu gönderildi, yanıt bekleniyor - Query ID: %s", queryID)

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
		log.Printf("[DEBUG] Sorgu yanıtı alındı - Query ID: %s", queryID)
		return result, nil
	case <-ctx.Done():
		log.Printf("[ERROR] Context iptal edildi - Query ID: %s, Hata: %v", queryID, ctx.Err())
		return nil, ctx.Err()
	case <-time.After(10 * time.Second): // 10 saniye timeout
		log.Printf("[ERROR] Sorgu zaman aşımına uğradı - Query ID: %s", queryID)
		return nil, fmt.Errorf("sorgu zaman aşımına uğradı")
	}
}

// SendSystemMetrics, agent'dan sistem metriklerini alır
func (s *Server) SendSystemMetrics(ctx context.Context, req *pb.SystemMetricsRequest) (*pb.SystemMetricsResponse, error) {
	log.Printf("[INFO] SendSystemMetrics başladı - basitleştirilmiş yaklaşım: agent_id=%s", req.AgentId)

	// Agent ID'yi standart formata getir
	agentID := req.AgentId
	if !strings.HasPrefix(agentID, "agent_") {
		agentID = "agent_" + agentID
		log.Printf("[DEBUG] Agent ID düzeltildi: %s", agentID)
	}

	// Agent bağlantısını bul
	s.mu.RLock()
	agentConn, ok := s.agents[agentID]
	s.mu.RUnlock()

	if !ok {
		log.Printf("[ERROR] Agent bulunamadı: %s", agentID)
		return nil, fmt.Errorf("agent bulunamadı: %s", agentID)
	}

	if agentConn.Stream == nil {
		log.Printf("[ERROR] Agent stream bağlantısı yok: %s", agentID)
		return nil, fmt.Errorf("agent stream bağlantısı yok: %s", agentID)
	}

	// Metrics için bir kanal oluştur
	metricsChan := make(chan *pb.SystemMetrics, 1)
	errorChan := make(chan error, 1)

	// Unique bir query ID oluştur
	queryID := fmt.Sprintf("metrics_%d", time.Now().UnixNano())

	// Query sonucu için bir kanal oluştur
	resultChan := make(chan *pb.QueryResult, 1)
	s.queryMu.Lock()
	s.queryResult[queryID] = &QueryResponse{
		ResultChan: resultChan,
	}
	s.queryMu.Unlock()

	// İşlem bitince cleanup yap
	defer func() {
		s.queryMu.Lock()
		delete(s.queryResult, queryID)
		s.queryMu.Unlock()
		close(resultChan)
		close(metricsChan)
		close(errorChan)
	}()

	log.Printf("[INFO] Metrik almak için özel sorgu gönderiliyor - QueryID: %s", queryID)

	// Metrik almak için "get_system_metrics" adında özel bir sorgu gönder
	err := agentConn.Stream.Send(&pb.ServerMessage{
		Payload: &pb.ServerMessage_Query{
			Query: &pb.Query{
				QueryId:  queryID,
				Command:  "get_system_metrics",
				Database: "",
			},
		},
	})

	if err != nil {
		log.Printf("[ERROR] Metrik sorgusu gönderilemedi: %v", err)
		return nil, fmt.Errorf("metrik sorgusu gönderilemedi: %v", err)
	}

	log.Printf("[INFO] Metrik sorgusu gönderildi, yanıt bekleniyor... QueryID: %s", queryID)

	// 3 saniyelik timeout ayarla
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// Yanıtı bekle
	select {
	case result := <-resultChan:
		if result == nil {
			log.Printf("[ERROR] Boş metrik yanıtı alındı")
			return nil, fmt.Errorf("boş metrik yanıtı alındı")
		}

		log.Printf("[INFO] Metrik yanıtı alındı - QueryID: %s", queryID)

		// Result içerisindeki Any tipini Struct'a dönüştür
		var metricsStruct structpb.Struct
		if err := result.Result.UnmarshalTo(&metricsStruct); err != nil {
			log.Printf("[ERROR] Metrik yapısı çözümlenemedi: %v", err)
			return nil, fmt.Errorf("metrik yapısı çözümlenemedi: %v", err)
		}

		// Yanıtı oluştur ve döndür
		response := &pb.SystemMetricsResponse{
			Status: "success",
			Data:   &metricsStruct,
		}

		log.Printf("[INFO] Metrik yanıtı başarıyla döndürülüyor")
		return response, nil

	case <-ctx.Done():
		log.Printf("[ERROR] Metrik yanıtı beklerken timeout: %v", ctx.Err())
		return nil, ctx.Err()
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
	log.Printf("[DEBUG] Starting saveAlarmToDatabase for alarm ID: %s", event.AlarmId)

	// Veritabanı bağlantısını kontrol et
	if err := s.checkDatabaseConnection(); err != nil {
		log.Printf("[ERROR] Database connection check failed: %v", err)
		return fmt.Errorf("veritabanı bağlantı hatası: %v", err)
	}
	log.Printf("[DEBUG] Database connection check passed")

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
			created_at,
			database
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	// Zaman damgasını parse et
	timestamp, err := time.Parse(time.RFC3339, event.Timestamp)
	if err != nil {
		log.Printf("[WARN] Failed to parse timestamp %q: %v, using current time", event.Timestamp, err)
		timestamp = time.Now() // Parse edilemezse şu anki zamanı kullan
	} else {
		log.Printf("[DEBUG] Successfully parsed timestamp: %v", timestamp)
	}

	// UTC zamanını Türkiye saatine (UTC+3) çevir
	turkeyLoc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		log.Printf("[WARN] Failed to load Turkey timezone: %v, using UTC", err)
	} else {
		timestamp = timestamp.In(turkeyLoc)
		log.Printf("[DEBUG] Converted timestamp to Turkey time: %v", timestamp)
	}

	// Veritabanına kaydet
	if event.MetricName == "postgresql_slow_queries" {
		log.Printf("[DEBUG] Slow query alarm received - Agent: %s, Value: %s, Message: %s",
			event.AgentId, event.MetricValue, event.Message)

		// postgresql_slow_queries için database alanı boşsa varsayılan olarak "postgres" ata
		if event.Database == "" {
			log.Printf("[DEBUG] Setting default database 'postgres' for postgresql_slow_queries alarm")
			event.Database = "postgres"
		}
	}

	log.Printf("[DEBUG] Executing database insert for alarm ID: %s, Event ID: %s", event.AlarmId, event.Id)
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
		event.Database,
	)

	if err != nil {
		log.Printf("[ERROR] Failed to save alarm to database: %v", err)
		return fmt.Errorf("alarm veritabanına kaydedilemedi: %v", err)
	}

	log.Printf("[INFO] Successfully saved alarm to database - ID: %s, Metric: %s, Agent: %s",
		event.Id, event.MetricName, event.AgentId)
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
						"title": "Metric",
						"value": event.MetricName,
						"short": true,
					},
					{
						"title": "Value",
						"value": event.MetricValue,
						"short": true,
					},
					{
						"title": "Severity",
						"value": event.Severity,
						"short": true,
					},
					{
						"title": "Status",
						"value": event.Status,
						"short": true,
					},
					{
						"title": "Database",
						"value": func() string {
							if event.Database != "" {
								return event.Database
							}
							return "N/A"
						}(),
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
					<div class="detail-label">Veritabanı:</div>
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
		func() string {
			if event.Database != "" {
				return event.Database
			}
			return "N/A"
		}(),
		event.AlarmId,
		event.Id,
		event.Timestamp,
	)

	// Basit metin içeriği
	textBody := fmt.Sprintf("ClusterEye Alarm Bildirimi\n\n%s\n\n%s\n\nAgent: %s\nMetrik: %s\nDeğer: %s\nÖnem: %s\nDurum: %s\nVeritabanı: %s\nAlarm ID: %s\nOlay ID: %s\nZaman: %s",
		subject,
		event.Message,
		event.AgentId,
		event.MetricName,
		event.MetricValue,
		event.Severity,
		event.Status,
		func() string {
			if event.Database != "" {
				return event.Database
			}
			return "N/A"
		}(),
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
		"TotalVCPU":         mongoInfo.TotalVcpu,
		"TotalMemory":       mongoInfo.TotalMemory,
		"ConfigPath":        mongoInfo.ConfigPath,
	}

	var jsonData []byte

	// Hata kontrolünü düzgün yap
	if err == nil {
		// Mevcut kayıt var, güncelle
		log.Printf("MongoDB cluster için mevcut kayıt bulundu, güncelleniyor: %s (ID: %d)", mongoInfo.ClusterName, id)

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
		nodeChanged := false
		for i, node := range clusterData {
			nodeMap, ok := node.(map[string]interface{})
			if !ok {
				continue
			}

			// Hostname ve IP ile node eşleşmesi kontrol et
			if nodeMap["Hostname"] == mongoInfo.Hostname && nodeMap["IP"] == mongoInfo.Ip {
				// Sadece değişen alanları güncelle
				nodeFound = true

				// Değişiklikleri takip et
				for key, newValue := range mongoData {
					currentValue, exists := nodeMap[key]
					var hasChanged bool

					if !exists {
						// Değer mevcut değil, yeni alan ekleniyor
						hasChanged = true
						log.Printf("MongoDB node'da yeni alan eklendi: %s, %s", mongoInfo.Hostname, key)
					} else {
						// Mevcut değer ile yeni değeri karşılaştır
						// Numeric değerler için özel karşılaştırma yap
						hasChanged = !compareValues(currentValue, newValue)
						if hasChanged {
							log.Printf("MongoDB node'da değişiklik tespit edildi: %s, %s: %v -> %v",
								mongoInfo.Hostname, key, currentValue, newValue)
						}
					}

					if hasChanged {
						nodeMap[key] = newValue
						// NodeStatus, MongoStatus, FreeDisk gibi önemli alanlar değiştiyse işaretle
						if key == "NodeStatus" || key == "MongoStatus" || key == "FreeDisk" || key == "ReplicationLagSec" {
							nodeChanged = true
						}
					}
				}

				clusterData[i] = nodeMap
				break
			}
		}

		// Eğer node bulunamadıysa yeni ekle
		if !nodeFound {
			clusterData = append(clusterData, mongoData)
			nodeChanged = true
			log.Printf("Yeni MongoDB node eklendi: %s", mongoInfo.Hostname)
		}

		// Eğer önemli bir değişiklik yoksa veritabanını güncelleme
		if !nodeChanged {
			log.Printf("MongoDB node'da önemli bir değişiklik yok, güncelleme yapılmadı: %s", mongoInfo.Hostname)
			return nil
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

		log.Printf("MongoDB node bilgileri başarıyla güncellendi (önemli değişiklik nedeniyle)")
	} else if err == sql.ErrNoRows {
		// Kayıt bulunamadı, yeni kayıt oluştur
		log.Printf("MongoDB cluster için kayıt bulunamadı, yeni kayıt oluşturuluyor: %s", mongoInfo.ClusterName)

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
			ON CONFLICT (clustername) DO UPDATE SET
				jsondata = EXCLUDED.jsondata,
				updated_at = CURRENT_TIMESTAMP
		`

		_, err = s.db.ExecContext(ctx, insertQuery, jsonData, mongoInfo.ClusterName)
		if err != nil {
			log.Printf("Veritabanı ekleme hatası: %v", err)
			return err
		}

		log.Printf("MongoDB node bilgileri başarıyla veritabanına kaydedildi (yeni kayıt)")
	} else {
		// Başka bir veritabanı hatası oluştu
		log.Printf("MongoDB cluster kayıt kontrolü sırasında hata: %v", err)
		return fmt.Errorf("veritabanı kontrol hatası: %v", err)
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

		// Sonucun içeriğini logla		// Struct'tan MongoLogAnalyzeResponse oluştur
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
func (s *Server) GetAlarms(ctx context.Context, onlyUnacknowledged bool, limit, offset int, severityFilter, metricFilter string, dateFrom, dateTo *time.Time) ([]map[string]interface{}, int64, error) {
	// Veritabanı bağlantısını kontrol et
	if err := s.checkDatabaseConnection(); err != nil {
		return nil, 0, fmt.Errorf("veritabanı bağlantı hatası: %v", err)
	}

	// Varsayılan limit kontrolü
	if limit <= 0 || limit > 1000 {
		limit = 50 // Varsayılan 50 kayıt
	}
	if offset < 0 {
		offset = 0
	}

	// WHERE koşullarını ve parametreleri hazırla
	var whereConditions []string
	var queryParams []interface{}
	paramIndex := 1

	// Sadece acknowledge edilmemiş alarmları getir
	if onlyUnacknowledged {
		whereConditions = append(whereConditions, fmt.Sprintf("acknowledged = $%d", paramIndex))
		queryParams = append(queryParams, false)
		paramIndex++
	}

	// Severity filtresi
	if severityFilter != "" {
		whereConditions = append(whereConditions, fmt.Sprintf("severity = $%d", paramIndex))
		queryParams = append(queryParams, severityFilter)
		paramIndex++
	}

	// Metric name filtresi
	if metricFilter != "" {
		whereConditions = append(whereConditions, fmt.Sprintf("metric_name = $%d", paramIndex))
		queryParams = append(queryParams, metricFilter)
		paramIndex++
	}

	// Tarih aralığı filtresi
	if dateFrom != nil {
		whereConditions = append(whereConditions, fmt.Sprintf("created_at >= $%d", paramIndex))
		queryParams = append(queryParams, *dateFrom)
		paramIndex++
	}
	if dateTo != nil {
		whereConditions = append(whereConditions, fmt.Sprintf("created_at <= $%d", paramIndex))
		queryParams = append(queryParams, *dateTo)
		paramIndex++
	}

	// WHERE clause oluştur
	whereClause := ""
	if len(whereConditions) > 0 {
		whereClause = " WHERE " + strings.Join(whereConditions, " AND ")
	}

	// Önce toplam kayıt sayısını al (count query)
	countQuery := "SELECT COUNT(*) FROM alarms" + whereClause
	var totalCount int64
	err := s.db.QueryRowContext(ctx, countQuery, queryParams...).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("toplam alarm sayısı alınamadı: %v", err)
	}

	// Ana SQL sorgusu
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
			acknowledged,
			database
		FROM alarms` + whereClause + `
		ORDER BY created_at DESC
		LIMIT $` + fmt.Sprintf("%d", paramIndex) + ` OFFSET $` + fmt.Sprintf("%d", paramIndex+1)

	// LIMIT ve OFFSET parametrelerini ekle
	queryParams = append(queryParams, limit, offset)

	log.Printf("Alarm sorgusu - Limit: %d, Offset: %d, Total: %d, Filters: severity=%s, metric=%s",
		limit, offset, totalCount, severityFilter, metricFilter)

	// Sorguyu çalıştır
	rows, err := s.db.QueryContext(ctx, query, queryParams...)
	if err != nil {
		return nil, 0, fmt.Errorf("alarm verileri çekilemedi: %v", err)
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
			database     sql.NullString // NULL değerleri kabul edebilmesi için NullString kullanıyoruz
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
			&database,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("satır okuma hatası: %v", err)
		}

		// Database değerini kontrolle ekle
		var databaseValue string
		if database.Valid {
			databaseValue = database.String
		} else {
			databaseValue = "" // Eğer NULL ise boş string atanır
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
			"database":     databaseValue,
		}

		alarms = append(alarms, alarm)
	}

	// Satır okuma hatası kontrolü
	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("satır okuma hatası: %v", err)
	}

	return alarms, totalCount, nil
}

// GetAlarmsStatus, alarm verilerini döndürür
func (s *Server) GetAlarmsStatus(ctx context.Context, _ *structpb.Struct) (*structpb.Value, error) {
	// Alarmları getir - Son 100 alarmı al
	alarms, _, err := s.GetAlarms(ctx, false, 100, 0, "", "", nil, nil)
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

// ReadPostgresConfig, belirtilen PostgreSQL config dosyasını okur
func (s *Server) ReadPostgresConfig(ctx context.Context, req *pb.PostgresConfigRequest) (*pb.PostgresConfigResponse, error) {
	// Agent ID'yi kontrol et
	agentID := req.AgentId
	if agentID == "" {
		return nil, fmt.Errorf("agent_id gerekli")
	}

	// Config dosya yolunu kontrol et
	configPath := req.ConfigPath
	if configPath == "" {
		return nil, fmt.Errorf("config_path gerekli")
	}

	// Config dosyasını okuma isteğini gönder
	return s.sendPostgresConfigQuery(ctx, agentID, configPath)
}

// sendPostgresConfigQuery, agent'a PostgreSQL config dosyasını okuması için sorgu gönderir
func (s *Server) sendPostgresConfigQuery(ctx context.Context, agentID, configPath string) (*pb.PostgresConfigResponse, error) {
	// Agent'ın bağlı olup olmadığını kontrol et
	s.mu.RLock()
	agent, agentExists := s.agents[agentID]
	s.mu.RUnlock()

	if !agentExists || agent == nil {
		return nil, fmt.Errorf("agent bulunamadı veya bağlantı kapalı: %s", agentID)
	}

	// configPath boş mu kontrol et
	if configPath == "" {
		return nil, fmt.Errorf("config dosya yolu boş olamaz")
	}

	// PostgreSQL config okuması için bir komut oluştur
	command := fmt.Sprintf("read_postgres_config|%s", configPath)
	queryID := fmt.Sprintf("postgres_config_%d", time.Now().UnixNano())

	log.Printf("PostgreSQL config okuması için komut: %s", command)

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
		log.Printf("PostgreSQL config sorgusu gönderilemedi - Hata: %v", err)
		return nil, fmt.Errorf("sorgu gönderilemedi: %v", err)
	}

	log.Printf("PostgreSQL config için sorgu gönderildi - Agent: %s, Path: %s, QueryID: %s",
		agentID, configPath, queryID)

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

			// Struct ayrıştırma başarısız olursa, PostgresConfigResponse olarak dene
			var configResponse pb.PostgresConfigResponse
			if err := result.Result.UnmarshalTo(&configResponse); err != nil {
				log.Printf("PostgresConfigResponse ayrıştırma hatası: %v", err)
				return nil, fmt.Errorf("sonuç ayrıştırma hatası: %v", err)
			}

			log.Printf("Doğrudan PostgresConfigResponse'a başarıyla ayrıştırıldı - Config girişleri: %d", len(configResponse.Configurations))
			return &configResponse, nil
		}

		// Struct'tan PostgresConfigResponse oluştur
		configEntries := make([]*pb.PostgresConfigEntry, 0)
		entriesValue, ok := resultStruct.Fields["configurations"]
		if ok && entriesValue != nil && entriesValue.GetListValue() != nil {
			for _, entryValue := range entriesValue.GetListValue().Values {
				if entryValue.GetStructValue() != nil {
					entryStruct := entryValue.GetStructValue()

					// Config giriş değerlerini al
					parameter := entryStruct.Fields["parameter"].GetStringValue()
					value := entryStruct.Fields["value"].GetStringValue()
					description := entryStruct.Fields["description"].GetStringValue()
					isDefault := entryStruct.Fields["is_default"].GetBoolValue()
					category := entryStruct.Fields["category"].GetStringValue()

					// PostgresConfigEntry oluştur
					configEntry := &pb.PostgresConfigEntry{
						Parameter:   parameter,
						Value:       value,
						Description: description,
						IsDefault:   isDefault,
						Category:    category,
					}

					configEntries = append(configEntries, configEntry)
				}
			}
		}

		log.Printf("Struct'tan oluşturulan config girişleri sayısı: %d", len(configEntries))

		// Config dosya yolunu al
		configPathValue := ""
		if pathValue, exists := resultStruct.Fields["config_path"]; exists {
			configPathValue = pathValue.GetStringValue()
		}

		return &pb.PostgresConfigResponse{
			Status:         "success",
			ConfigPath:     configPathValue,
			Configurations: configEntries,
		}, nil

	case <-ctx.Done():
		// Context iptal edildi veya zaman aşımına uğradı
		log.Printf("Context iptal edildi veya zaman aşımına uğradı - QueryID: %s", queryID)
		return nil, ctx.Err()
	}
}

// ReportVersion, agent'ın versiyon bilgilerini işler
func (s *Server) ReportVersion(ctx context.Context, req *pb.ReportVersionRequest) (*pb.ReportVersionResponse, error) {
	log.Printf("ReportVersion metodu çağrıldı - Agent ID: %s", req.AgentId)

	// Veritabanı bağlantısını kontrol et
	if err := s.checkDatabaseConnection(); err != nil {
		log.Printf("Veritabanı bağlantı hatası: %v", err)
		return &pb.ReportVersionResponse{
			Status: "error",
		}, fmt.Errorf("veritabanı bağlantı hatası: %v", err)
	}

	// Versiyon bilgilerini logla
	versionInfo := req.VersionInfo
	log.Printf("Agent versiyon bilgileri alındı:")
	log.Printf("  Version: %s", versionInfo.Version)
	log.Printf("  Platform: %s", versionInfo.Platform)
	log.Printf("  Architecture: %s", versionInfo.Architecture)
	log.Printf("  Hostname: %s", versionInfo.Hostname)
	log.Printf("  OS Version: %s", versionInfo.OsVersion)
	log.Printf("  Go Version: %s", versionInfo.GoVersion)

	// Versiyon bilgilerini veritabanına kaydet
	query := `
		INSERT INTO agent_versions (
			agent_id,
			version,
			platform,
			architecture,
			hostname,
			os_version,
			go_version,
			reported_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, CURRENT_TIMESTAMP)
		ON CONFLICT (agent_id) DO UPDATE SET
			version = EXCLUDED.version,
			platform = EXCLUDED.platform,
			architecture = EXCLUDED.architecture,
			hostname = EXCLUDED.hostname,
			os_version = EXCLUDED.os_version,
			go_version = EXCLUDED.go_version,
			reported_at = CURRENT_TIMESTAMP
	`

	_, err := s.db.ExecContext(ctx, query,
		req.AgentId,
		versionInfo.Version,
		versionInfo.Platform,
		versionInfo.Architecture,
		versionInfo.Hostname,
		versionInfo.OsVersion,
		versionInfo.GoVersion,
	)

	if err != nil {
		log.Printf("Versiyon bilgileri kaydedilemedi: %v", err)
		return &pb.ReportVersionResponse{
			Status: "error",
		}, fmt.Errorf("versiyon bilgileri kaydedilemedi: %v", err)
	}

	log.Printf("Agent versiyon bilgileri başarıyla kaydedildi - Agent ID: %s", req.AgentId)
	return &pb.ReportVersionResponse{
		Status: "success",
	}, nil
}

// GetAgentVersions, veritabanından agent versiyon bilgilerini çeker
func (s *Server) GetAgentVersions(ctx context.Context) ([]map[string]interface{}, error) {
	// Veritabanı bağlantısını kontrol et
	if err := s.checkDatabaseConnection(); err != nil {
		return nil, fmt.Errorf("veritabanı bağlantı hatası: %v", err)
	}

	// SQL sorgusu
	query := `
		SELECT 
			agent_id,
			version,
			platform,
			architecture,
			hostname,
			os_version,
			go_version,
			reported_at,
			created_at,
			updated_at
		FROM agent_versions
		ORDER BY reported_at DESC
	`

	// Sorguyu çalıştır
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("versiyon bilgileri çekilemedi: %v", err)
	}
	defer rows.Close()

	// Sonuçları topla
	versions := make([]map[string]interface{}, 0)
	for rows.Next() {
		var (
			agentID      string
			version      string
			platform     string
			architecture string
			hostname     string
			osVersion    string
			goVersion    string
			reportedAt   time.Time
			createdAt    time.Time
			updatedAt    time.Time
		)

		// Satırı oku
		err := rows.Scan(
			&agentID,
			&version,
			&platform,
			&architecture,
			&hostname,
			&osVersion,
			&goVersion,
			&reportedAt,
			&createdAt,
			&updatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("satır okuma hatası: %v", err)
		}

		// Her versiyonu map olarak oluştur
		versionInfo := map[string]interface{}{
			"agent_id":     agentID,
			"version":      version,
			"platform":     platform,
			"architecture": architecture,
			"hostname":     hostname,
			"os_version":   osVersion,
			"go_version":   goVersion,
			"reported_at":  reportedAt.Format(time.RFC3339),
			"created_at":   createdAt.Format(time.RFC3339),
			"updated_at":   updatedAt.Format(time.RFC3339),
		}

		versions = append(versions, versionInfo)
	}

	// Satır okuma hatası kontrolü
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("satır okuma hatası: %v", err)
	}

	return versions, nil
}

// PromoteMongoToPrimary, MongoDB node'unu primary'ye yükseltir
func (s *Server) PromoteMongoToPrimary(ctx context.Context, req *pb.MongoPromotePrimaryRequest) (*pb.MongoPromotePrimaryResponse, error) {
	// Job oluştur
	job := &pb.Job{
		JobId:     req.JobId,
		Type:      pb.JobType_JOB_TYPE_MONGO_PROMOTE_PRIMARY,
		Status:    pb.JobStatus_JOB_STATUS_PENDING,
		AgentId:   req.AgentId,
		CreatedAt: timestamppb.Now(),
		UpdatedAt: timestamppb.Now(),
		Parameters: map[string]string{
			"node_hostname": req.NodeHostname,
			"port":          fmt.Sprintf("%d", req.Port),
			"replica_set":   req.ReplicaSet,
			"node_status":   req.NodeStatus, // Node status'ını ekle
		},
	}

	// Job'ı kaydet
	s.jobMu.Lock()
	s.jobs[job.JobId] = job
	s.jobMu.Unlock()

	// İlgili agent'ı bul
	s.mu.RLock()
	agent, exists := s.agents[req.AgentId]
	s.mu.RUnlock()

	if !exists {
		job.Status = pb.JobStatus_JOB_STATUS_FAILED
		job.ErrorMessage = "Agent bulunamadı"
		job.UpdatedAt = timestamppb.Now()
		return &pb.MongoPromotePrimaryResponse{
			JobId:        job.JobId,
			Status:       job.Status,
			ErrorMessage: job.ErrorMessage,
		}, fmt.Errorf("agent bulunamadı: %s", req.AgentId)
	}

	// Job'ı veritabanına kaydet
	if err := s.saveJobToDatabase(ctx, job); err != nil {
		job.Status = pb.JobStatus_JOB_STATUS_FAILED
		job.ErrorMessage = fmt.Sprintf("Job veritabanına kaydedilemedi: %v", err)
		job.UpdatedAt = timestamppb.Now()
		return &pb.MongoPromotePrimaryResponse{
			JobId:        job.JobId,
			Status:       job.Status,
			ErrorMessage: job.ErrorMessage,
		}, err
	}

	// Job'ı çalıştır
	go func() {
		job.Status = pb.JobStatus_JOB_STATUS_RUNNING

		// MongoDB komutu oluştur - node_status'a göre farklı komut gönder
		var command string
		if req.NodeStatus == "PRIMARY" {
			// Eğer node primary ise, primary'i step down yapalım
			command = "rs.stepDown()"
		} else {
			// Secondary node durumunda bir şey yapmaya gerek yok (freeze endpoint'i yoluyla hallolacak)
			command = fmt.Sprintf("db.hello() // Node %s secondary durumunda", req.NodeHostname)
		}

		// Agent'a promote isteği gönder
		err := agent.Stream.Send(&pb.ServerMessage{
			Payload: &pb.ServerMessage_Query{
				Query: &pb.Query{
					QueryId: job.JobId,
					Command: command,
				},
			},
		})

		if err != nil {
			job.Status = pb.JobStatus_JOB_STATUS_FAILED
			job.ErrorMessage = fmt.Sprintf("Agent'a istek gönderilemedi: %v", err)
		} else {
			job.Status = pb.JobStatus_JOB_STATUS_COMPLETED
			job.Result = "Primary promotion request sent successfully"
		}

		job.UpdatedAt = timestamppb.Now()
		s.updateJobInDatabase(context.Background(), job)
	}()

	return &pb.MongoPromotePrimaryResponse{
		JobId:  job.JobId,
		Status: pb.JobStatus_JOB_STATUS_PENDING,
	}, nil
}

// PromotePostgresToMaster, PostgreSQL node'unu master'a yükseltir
func (s *Server) PromotePostgresToMaster(ctx context.Context, req *pb.PostgresPromoteMasterRequest) (*pb.PostgresPromoteMasterResponse, error) {
	log.Printf("PromotePostgresToMaster çağrıldı - Agent ID: %s, Node: %s, Data Directory: %s",
		req.AgentId, req.NodeHostname, req.DataDirectory)

	// Process log takibi için metadata oluştur
	metadata := map[string]string{
		"node_hostname":  req.NodeHostname,
		"data_directory": req.DataDirectory,
		"job_id":         req.JobId,
	}

	// Job oluştur
	job := &pb.Job{
		JobId:     req.JobId,
		Type:      pb.JobType_JOB_TYPE_POSTGRES_PROMOTE_MASTER,
		Status:    pb.JobStatus_JOB_STATUS_PENDING,
		AgentId:   req.AgentId,
		CreatedAt: timestamppb.Now(),
		UpdatedAt: timestamppb.Now(),
		Parameters: map[string]string{
			"node_hostname":  req.NodeHostname,
			"data_directory": req.DataDirectory,
			"process_id":     req.JobId, // Process ID olarak job ID'yi kullan
		},
	}

	// Job'ı kaydet
	s.jobMu.Lock()
	s.jobs[job.JobId] = job
	s.jobMu.Unlock()

	// İlgili agent'ı bul
	s.mu.RLock()
	agent, exists := s.agents[req.AgentId]
	s.mu.RUnlock()

	if !exists {
		job.Status = pb.JobStatus_JOB_STATUS_FAILED
		job.ErrorMessage = "Agent bulunamadı"
		job.UpdatedAt = timestamppb.Now()

		// Process log oluştur - hata durumu
		processLog := &pb.ProcessLogUpdate{
			AgentId:      req.AgentId,
			ProcessId:    req.JobId,
			ProcessType:  "postgresql_promotion",
			Status:       "failed",
			LogMessages:  []string{"Agent bulunamadı, işlem başlatılamadı"},
			ElapsedTimeS: 0,
			UpdatedAt:    time.Now().Format(time.RFC3339),
			Metadata:     metadata,
		}
		s.saveProcessLogs(ctx, processLog)

		return &pb.PostgresPromoteMasterResponse{
			JobId:        job.JobId,
			Status:       job.Status,
			ErrorMessage: job.ErrorMessage,
		}, fmt.Errorf("agent bulunamadı: %s", req.AgentId)
	}

	// Job'ı veritabanına kaydet
	if err := s.saveJobToDatabase(ctx, job); err != nil {
		job.Status = pb.JobStatus_JOB_STATUS_FAILED
		job.ErrorMessage = fmt.Sprintf("Job veritabanına kaydedilemedi: %v", err)
		job.UpdatedAt = timestamppb.Now()

		// Process log oluştur - veritabanı hatası
		processLog := &pb.ProcessLogUpdate{
			AgentId:      req.AgentId,
			ProcessId:    req.JobId,
			ProcessType:  "postgresql_promotion",
			Status:       "failed",
			LogMessages:  []string{fmt.Sprintf("Job veritabanına kaydedilemedi: %v", err)},
			ElapsedTimeS: 0,
			UpdatedAt:    time.Now().Format(time.RFC3339),
			Metadata:     metadata,
		}
		s.saveProcessLogs(ctx, processLog)

		return &pb.PostgresPromoteMasterResponse{
			JobId:        job.JobId,
			Status:       job.Status,
			ErrorMessage: job.ErrorMessage,
		}, err
	}

	// Process log başlangıç kaydı oluştur
	startLog := &pb.ProcessLogUpdate{
		AgentId:      req.AgentId,
		ProcessId:    req.JobId,
		ProcessType:  "postgresql_promotion",
		Status:       "running",
		LogMessages:  []string{fmt.Sprintf("PostgreSQL promotion işlemi başlatılıyor - Node: %s", req.NodeHostname)},
		ElapsedTimeS: 0,
		UpdatedAt:    time.Now().Format(time.RFC3339),
		Metadata:     metadata,
	}
	s.saveProcessLogs(ctx, startLog)

	// Job'ı çalıştır
	go func() {
		job.Status = pb.JobStatus_JOB_STATUS_RUNNING
		job.UpdatedAt = timestamppb.Now()
		s.updateJobInDatabase(context.Background(), job)

		// Yeni komut formatı: "postgres_promote|data_dir|process_id"
		// Agent tarafındaki ProcessLogger'ı etkinleştirmek için process_id gönderiyoruz
		command := fmt.Sprintf("postgres_promote|%s|%s", req.DataDirectory, req.JobId)

		// Agent'a promote isteği gönder
		err := agent.Stream.Send(&pb.ServerMessage{
			Payload: &pb.ServerMessage_Query{
				Query: &pb.Query{
					QueryId: job.JobId,
					Command: command,
				},
			},
		})

		if err != nil {
			job.Status = pb.JobStatus_JOB_STATUS_FAILED
			job.ErrorMessage = fmt.Sprintf("Agent'a istek gönderilemedi: %v", err)
			job.UpdatedAt = timestamppb.Now()
			s.updateJobInDatabase(context.Background(), job)

			// Hata durumu process log'u
			errorLog := &pb.ProcessLogUpdate{
				AgentId:      req.AgentId,
				ProcessId:    req.JobId,
				ProcessType:  "postgresql_promotion",
				Status:       "failed",
				LogMessages:  []string{fmt.Sprintf("Agent'a istek gönderilemedi: %v", err)},
				ElapsedTimeS: 0,
				UpdatedAt:    time.Now().Format(time.RFC3339),
				Metadata:     metadata,
			}
			s.saveProcessLogs(context.Background(), errorLog)
		} else {
			// Başarılı başlatma durumu
			log.Printf("PostgreSQL promotion işlemi agent'a iletildi - Job ID: %s", job.JobId)

			// Job'ı IN_PROGRESS olarak işaretle
			// Agent ProcessLogger ile ilerleyişi bildirecek, burada bir şey yapmamıza gerek yok

			// Tamamlandı olarak işaretleme işlemini artık agent tarafından gelen
			// son log mesajı (completed statüsünde) ile yapacağız.
		}
	}()

	return &pb.PostgresPromoteMasterResponse{
		JobId:  job.JobId,
		Status: pb.JobStatus_JOB_STATUS_PENDING,
	}, nil
}

// GetJob, belirli bir job'ın detaylarını getirir
func (s *Server) GetJob(ctx context.Context, req *pb.GetJobRequest) (*pb.GetJobResponse, error) {
	s.jobMu.RLock()
	job, exists := s.jobs[req.JobId]
	s.jobMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("job bulunamadı: %s", req.JobId)
	}

	return &pb.GetJobResponse{
		Job: job,
	}, nil
}

// ListJobs, job listesini veritabanından doğrudan sorgular ve getirir
func (s *Server) ListJobs(ctx context.Context, req *pb.ListJobsRequest) (*pb.ListJobsResponse, error) {
	log.Printf("[DEBUG] ListJobs çağrıldı - Parametreler: agent_id=%s, status=%v, type=%v, limit=%d, offset=%d",
		req.AgentId, req.Status, req.Type, req.Limit, req.Offset)

	// Veritabanı bağlantısını kontrol et
	if err := s.checkDatabaseConnection(); err != nil {
		return nil, fmt.Errorf("veritabanı bağlantı hatası: %v", err)
	}

	// Sorgu için parametreleri hazırla
	var queryParams []interface{}
	var conditions []string

	// Temel sorgu oluştur
	baseQuery := `
		SELECT 
			job_id, type, status, agent_id, 
			created_at, updated_at, error_message, 
			parameters, result
		FROM jobs
	`

	// WHERE koşulları oluştur
	paramCount := 1 // PostgreSQL parametreleri $1, $2, ... şeklinde başlar

	// Agent ID filtresi ekle
	if req.AgentId != "" {
		conditions = append(conditions, fmt.Sprintf("agent_id = $%d", paramCount))
		queryParams = append(queryParams, req.AgentId)
		paramCount++
	}

	// Status filtresi ekle
	if req.Status != pb.JobStatus_JOB_STATUS_UNKNOWN {
		conditions = append(conditions, fmt.Sprintf("status = $%d", paramCount))
		queryParams = append(queryParams, req.Status.String())
		paramCount++
	}

	// Type filtresi ekle
	if req.Type != pb.JobType_JOB_TYPE_UNKNOWN {
		conditions = append(conditions, fmt.Sprintf("type = $%d", paramCount))
		queryParams = append(queryParams, req.Type.String())
		paramCount++
	}

	// WHERE koşullarını SQL sorgusuna ekle
	query := baseQuery
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	// Toplam kayıt sayısını almak için COUNT sorgusu
	countQuery := "SELECT COUNT(*) FROM jobs"
	if len(conditions) > 0 {
		countQuery += " WHERE " + strings.Join(conditions, " AND ")
	}

	// Önce toplam kayıt sayısını al
	var total int32
	err := s.db.QueryRowContext(ctx, countQuery, queryParams...).Scan(&total)
	if err != nil {
		log.Printf("[ERROR] Job sayısı alınamadı: %v", err)
		return nil, fmt.Errorf("job sayısı alınamadı: %v", err)
	}
	log.Printf("[DEBUG] Filtrelere göre toplam job sayısı: %d", total)

	// Sıralama ve limit ekle
	query += " ORDER BY created_at DESC"

	// Varsayılan değerler
	limit := req.Limit
	if limit <= 0 {
		limit = 10 // Varsayılan limit
	}

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", paramCount, paramCount+1)
	queryParams = append(queryParams, limit, offset)

	log.Printf("[DEBUG] SQL sorgusu: %s", query)
	log.Printf("[DEBUG] Parametreler: %v", queryParams)

	// Asıl sorguyu çalıştır
	rows, err := s.db.QueryContext(ctx, query, queryParams...)
	if err != nil {
		log.Printf("[ERROR] Job sorgusu çalıştırılamadı: %v", err)
		return nil, fmt.Errorf("job sorgusu çalıştırılamadı: %v", err)
	}
	defer rows.Close()

	// Sonuçları işle
	var jobs []*pb.Job
	for rows.Next() {
		var (
			jobID        string
			jobType      string
			jobStatus    string
			agentID      string
			createdAt    time.Time
			updatedAt    time.Time
			errorMessage sql.NullString
			parameters   []byte // JSON formatında
			result       sql.NullString
		)

		// Satırı oku
		if err := rows.Scan(
			&jobID, &jobType, &jobStatus, &agentID,
			&createdAt, &updatedAt, &errorMessage,
			&parameters, &result,
		); err != nil {
			log.Printf("[ERROR] Job kaydı okunamadı: %v", err)
			continue
		}

		// Parameters JSON'ı çözümle
		var paramsMap map[string]string
		if err := json.Unmarshal(parameters, &paramsMap); err != nil {
			log.Printf("[ERROR] Job parametreleri çözümlenemedi (%s): %v", jobID, err)
			paramsMap = make(map[string]string) // Boş map kullan
		}

		// JobType ve JobStatus enum değerlerini çözümle
		jobTypeEnum, ok := pb.JobType_value[jobType]
		if !ok {
			log.Printf("[WARN] Bilinmeyen job tipi: %s, varsayılan olarak JOB_TYPE_UNKNOWN kullanılıyor", jobType)
			jobTypeEnum = int32(pb.JobType_JOB_TYPE_UNKNOWN)
		}

		jobStatusEnum, ok := pb.JobStatus_value[jobStatus]
		if !ok {
			log.Printf("[WARN] Bilinmeyen job durumu: %s, varsayılan olarak JOB_STATUS_UNKNOWN kullanılıyor", jobStatus)
			jobStatusEnum = int32(pb.JobStatus_JOB_STATUS_UNKNOWN)
		}

		// pb.Job nesnesi oluştur
		job := &pb.Job{
			JobId:      jobID,
			Type:       pb.JobType(jobTypeEnum),
			Status:     pb.JobStatus(jobStatusEnum),
			AgentId:    agentID,
			CreatedAt:  timestamppb.New(createdAt),
			UpdatedAt:  timestamppb.New(updatedAt),
			Parameters: paramsMap,
		}

		// Null olabilecek alanları işle
		if errorMessage.Valid {
			job.ErrorMessage = errorMessage.String
		}

		if result.Valid {
			job.Result = result.String
		}

		// Job'ı listeye ekle
		jobs = append(jobs, job)
	}

	// Rows.Err() kontrolü
	if err := rows.Err(); err != nil {
		log.Printf("[ERROR] Job kayıtları okunurken hata: %v", err)
		return nil, fmt.Errorf("job kayıtları okunurken hata: %v", err)
	}

	log.Printf("[DEBUG] Veritabanından %d job kaydı döndürülüyor (toplam: %d)", len(jobs), total)

	return &pb.ListJobsResponse{
		Jobs:  jobs,
		Total: total,
	}, nil
}

// saveJobToDatabase, job'ı veritabanına kaydeder
func (s *Server) saveJobToDatabase(ctx context.Context, job *pb.Job) error {
	query := `
		INSERT INTO jobs (
			job_id,
			type,
			status,
			agent_id,
			created_at,
			updated_at,
			error_message,
			parameters,
			result
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	// map[string]string tipini JSON'a çevir
	paramsJSON, err := json.Marshal(job.Parameters)
	if err != nil {
		return fmt.Errorf("parameters JSON'a çevrilemedi: %v", err)
	}

	_, err = s.db.ExecContext(ctx, query,
		job.JobId,
		job.Type.String(),
		job.Status.String(),
		job.AgentId,
		job.CreatedAt.AsTime(),
		job.UpdatedAt.AsTime(),
		job.ErrorMessage,
		paramsJSON, // JSON formatında parameterler
		job.Result,
	)

	return err
}

// updateJobInDatabase, job'ı veritabanında günceller
func (s *Server) updateJobInDatabase(ctx context.Context, job *pb.Job) error {
	query := `
		UPDATE jobs SET
			status = $1,
			updated_at = $2,
			error_message = $3,
			result = $4
		WHERE job_id = $5
	`

	_, err := s.db.ExecContext(ctx, query,
		job.Status.String(),
		job.UpdatedAt.AsTime(),
		job.ErrorMessage,
		job.Result,
		job.JobId,
	)

	return err
}

// CreateJob, genel bir job oluşturur ve veritabanına kaydeder
func (s *Server) CreateJob(ctx context.Context, job *pb.Job) error {
	// Job'ı memory'de kaydet
	s.jobMu.Lock()
	s.jobs[job.JobId] = job
	s.jobMu.Unlock()

	// Veritabanına kaydet
	if err := s.saveJobToDatabase(ctx, job); err != nil {
		return err
	}

	// Job tipi ve parametrelere göre uygun işlemi başlat
	go func() {
		// Job durumunu çalışıyor olarak güncelle
		job.Status = pb.JobStatus_JOB_STATUS_RUNNING
		job.UpdatedAt = timestamppb.Now()
		s.updateJobInDatabase(context.Background(), job)

		// İlgili agent'ı bul
		s.mu.RLock()
		agent, exists := s.agents[job.AgentId]
		s.mu.RUnlock()

		if !exists {
			job.Status = pb.JobStatus_JOB_STATUS_FAILED
			job.ErrorMessage = "Agent bulunamadı"
			job.UpdatedAt = timestamppb.Now()
			s.updateJobInDatabase(context.Background(), job)
			return
		}

		var err error
		var command string

		// Job tipine göre işlemi belirle
		switch job.Type {
		case pb.JobType_JOB_TYPE_MONGO_PROMOTE_PRIMARY:
			nodeHostname := job.Parameters["node_hostname"]
			replicaSet := job.Parameters["replica_set"]
			nodeStatus := job.Parameters["node_status"]
			if nodeHostname == "" || replicaSet == "" {
				job.Status = pb.JobStatus_JOB_STATUS_FAILED
				job.ErrorMessage = "Eksik parametreler: node_hostname veya replica_set"
				break
			}

			// Node status'a göre komutu belirle
			if nodeStatus == "primary" {
				// Eğer node zaten primary ise, farklı bir komut gönder
				command = fmt.Sprintf("db.isMaster() // Node %s zaten primary", nodeHostname)
			} else {
				// Secondary node'u primary'ye yükselt
				command = fmt.Sprintf("rs.stepDown('%s')", nodeHostname)
			}

		case pb.JobType_JOB_TYPE_POSTGRES_PROMOTE_MASTER:
			dataDir := job.Parameters["data_directory"]
			if dataDir == "" {
				job.Status = pb.JobStatus_JOB_STATUS_FAILED
				job.ErrorMessage = "Eksik parametre: data_directory"
				break
			}
			command = fmt.Sprintf("pg_ctl promote -D %s", dataDir)

		default:
			job.Status = pb.JobStatus_JOB_STATUS_FAILED
			job.ErrorMessage = fmt.Sprintf("Desteklenmeyen job tipi: %s", job.Type.String())
		}

		if job.Status == pb.JobStatus_JOB_STATUS_FAILED {
			job.UpdatedAt = timestamppb.Now()
			s.updateJobInDatabase(context.Background(), job)
			return
		}

		// Agent'a komutu gönder
		err = agent.Stream.Send(&pb.ServerMessage{
			Payload: &pb.ServerMessage_Query{
				Query: &pb.Query{
					QueryId: job.JobId,
					Command: command,
				},
			},
		})

		if err != nil {
			job.Status = pb.JobStatus_JOB_STATUS_FAILED
			job.ErrorMessage = fmt.Sprintf("Agent'a istek gönderilemedi: %v", err)
		} else {
			job.Status = pb.JobStatus_JOB_STATUS_COMPLETED
			job.Result = "Job request sent successfully"
		}

		job.UpdatedAt = timestamppb.Now()
		s.updateJobInDatabase(context.Background(), job)
	}()

	return nil
}

// FreezeMongoSecondary, MongoDB secondary node'larını belirli bir süre için dondurur
func (s *Server) FreezeMongoSecondary(ctx context.Context, req *pb.MongoFreezeSecondaryRequest) (*pb.MongoFreezeSecondaryResponse, error) {
	// Job oluştur
	job := &pb.Job{
		JobId:     req.JobId,
		Type:      pb.JobType_JOB_TYPE_MONGO_FREEZE_SECONDARY,
		Status:    pb.JobStatus_JOB_STATUS_PENDING,
		AgentId:   req.AgentId,
		CreatedAt: timestamppb.Now(),
		UpdatedAt: timestamppb.Now(),
		Parameters: map[string]string{
			"node_hostname": req.NodeHostname,
			"port":          fmt.Sprintf("%d", req.Port),
			"replica_set":   req.ReplicaSet,
			"seconds":       fmt.Sprintf("%d", req.Seconds),
		},
	}

	// Job'ı kaydet
	s.jobMu.Lock()
	s.jobs[job.JobId] = job
	s.jobMu.Unlock()

	// İlgili agent'ı bul
	s.mu.RLock()
	agent, exists := s.agents[req.AgentId]
	s.mu.RUnlock()

	if !exists {
		job.Status = pb.JobStatus_JOB_STATUS_FAILED
		job.ErrorMessage = "Agent bulunamadı"
		job.UpdatedAt = timestamppb.Now()
		return &pb.MongoFreezeSecondaryResponse{
			JobId:        job.JobId,
			Status:       job.Status,
			ErrorMessage: job.ErrorMessage,
		}, fmt.Errorf("agent bulunamadı: %s", req.AgentId)
	}

	// Job'ı veritabanına kaydet
	if err := s.saveJobToDatabase(ctx, job); err != nil {
		job.Status = pb.JobStatus_JOB_STATUS_FAILED
		job.ErrorMessage = fmt.Sprintf("Job veritabanına kaydedilemedi: %v", err)
		job.UpdatedAt = timestamppb.Now()
		return &pb.MongoFreezeSecondaryResponse{
			JobId:        job.JobId,
			Status:       job.Status,
			ErrorMessage: job.ErrorMessage,
		}, err
	}

	// Job'ı çalıştır
	go func() {
		job.Status = pb.JobStatus_JOB_STATUS_RUNNING
		job.UpdatedAt = timestamppb.Now()
		s.updateJobInDatabase(context.Background(), job)

		// Seconds parametresini kontrol et, varsayılan 60
		seconds := 60
		if req.Seconds > 0 {
			seconds = int(req.Seconds)
		}

		// rs.freeze() komutu oluştur
		command := fmt.Sprintf("rs.freeze(%d)", seconds)

		// Agent'a freeze isteği gönder
		err := agent.Stream.Send(&pb.ServerMessage{
			Payload: &pb.ServerMessage_Query{
				Query: &pb.Query{
					QueryId: job.JobId,
					Command: command,
				},
			},
		})

		if err != nil {
			job.Status = pb.JobStatus_JOB_STATUS_FAILED
			job.ErrorMessage = fmt.Sprintf("Agent'a istek gönderilemedi: %v", err)
		} else {
			job.Status = pb.JobStatus_JOB_STATUS_COMPLETED
			job.Result = fmt.Sprintf("MongoDB node %s successfully frozen for %d seconds", req.NodeHostname, seconds)
		}

		job.UpdatedAt = timestamppb.Now()
		s.updateJobInDatabase(context.Background(), job)
	}()

	return &pb.MongoFreezeSecondaryResponse{
		JobId:  job.JobId,
		Status: pb.JobStatus_JOB_STATUS_PENDING,
	}, nil
}

// ExplainQuery, PostgreSQL sorgu planını EXPLAIN ANALYZE kullanarak getirir
func (s *Server) ExplainQuery(ctx context.Context, req *pb.ExplainQueryRequest) (*pb.ExplainQueryResponse, error) {
	log.Printf("ExplainQuery metodu çağrıldı - Agent ID: %s, Database: %s", req.AgentId, req.Database)

	// Agent ID'yi kontrol et
	agentID := req.AgentId
	if agentID == "" {
		return &pb.ExplainQueryResponse{
			Status:       "error",
			ErrorMessage: "agent_id boş olamaz",
		}, fmt.Errorf("agent_id boş olamaz")
	}

	// Sorguyu kontrol et
	query := req.Query
	if query == "" {
		return &pb.ExplainQueryResponse{
			Status:       "error",
			ErrorMessage: "sorgu boş olamaz",
		}, fmt.Errorf("sorgu boş olamaz")
	}

	// EXPLAIN ANALYZE ile sorguyu çevrele
	explainQuery := fmt.Sprintf("EXPLAIN (ANALYZE true, BUFFERS true, COSTS true, TIMING true, VERBOSE true, FORMAT TEXT) %s", query)

	// Unique bir sorgu ID'si oluştur
	queryID := fmt.Sprintf("explain_%d", time.Now().UnixNano())

	// Agent'a sorguyu gönder ve cevabı al
	result, err := s.SendQuery(ctx, agentID, queryID, explainQuery, req.Database)
	if err != nil {
		log.Printf("Sorgu planı alınırken hata: %v", err)
		return &pb.ExplainQueryResponse{
			Status:       "error",
			ErrorMessage: fmt.Sprintf("Sorgu planı alınırken hata: %v", err),
		}, err
	}

	// Sorgu sonucunu döndür
	if result.Result != nil {
		log.Printf("Sorgu sonucu alındı, type_url: %s", result.Result.TypeUrl)

		// Sonucu okunabilir formata dönüştür
		var resultStruct structpb.Struct
		if err := result.Result.UnmarshalTo(&resultStruct); err != nil {
			log.Printf("Sonuç structpb.Struct'a dönüştürülürken hata: %v", err)

			// Farklı bir yöntem deneyelim - doğrudan JSON string'e çevirmeyi deneyelim
			resultStr := string(result.Result.Value)
			if len(resultStr) > 0 {
				log.Printf("Result.Value doğrudan string olarak kullanılıyor: %d byte", len(resultStr))
				return &pb.ExplainQueryResponse{
					Status: "success",
					Plan:   resultStr,
				}, nil
			}

			return &pb.ExplainQueryResponse{
				Status:       "error",
				ErrorMessage: fmt.Sprintf("Sonuç dönüştürülürken hata: %v", err),
			}, err
		}

		// Struct'ı doğrudan JSON string'e dönüştür
		resultMap := resultStruct.AsMap()
		resultBytes, err := json.Marshal(resultMap)
		if err != nil {
			log.Printf("JSON dönüştürme hatası: %v", err)
			return &pb.ExplainQueryResponse{
				Status:       "error",
				ErrorMessage: fmt.Sprintf("JSON dönüştürme hatası: %v", err),
			}, err
		}

		// Eğer resultMap içinde özellikle "result" veya "explain" anahtarı varsa,
		// bunları kullanarak okunabilir bir açıklama formatı oluştur
		var planText string
		if explainResult, ok := resultMap["result"]; ok {
			// Sonuç "result" anahtarında ise
			if explainStr, ok := explainResult.(string); ok {
				planText = explainStr
			} else {
				// Eğer doğrudan string değilse JSON olarak dönüştür
				if explainBytes, err := json.MarshalIndent(explainResult, "", "  "); err == nil {
					planText = string(explainBytes)
				}
			}
		} else if explainResult, ok := resultMap["explain"]; ok {
			// Sonuç "explain" anahtarında ise
			if explainStr, ok := explainResult.(string); ok {
				planText = explainStr
			} else {
				// Eğer doğrudan string değilse JSON olarak dönüştür
				if explainBytes, err := json.MarshalIndent(explainResult, "", "  "); err == nil {
					planText = string(explainBytes)
				}
			}
		}

		// Eğer özel format bulunamadıysa, tüm JSON formatını kullan
		if planText == "" {
			planText = string(resultBytes)
		}

		// JSON string'i doğrudan plan alanında kullan
		return &pb.ExplainQueryResponse{
			Status: "success",
			Plan:   planText,
		}, nil
	}

	// Sonuç boş ise hata döndür
	return &pb.ExplainQueryResponse{
		Status:       "error",
		ErrorMessage: "Sorgu planı alınamadı",
	}, fmt.Errorf("sorgu planı alınamadı")
}

// ExplainMongoQuery, MongoDB sorgu planını explain() kullanarak getirir
func (s *Server) ExplainMongoQuery(ctx context.Context, req *pb.ExplainQueryRequest) (*pb.ExplainQueryResponse, error) {
	log.Printf("[INFO] ExplainMongoQuery çağrıldı - Agent ID: %s, Database: %s", req.AgentId, req.Database)

	// Ham sorguyu loglayalım
	log.Printf("[DEBUG] Ham sorgu (JSON): %s", req.Query)

	// Agent ID'yi kontrol et
	agentID := req.AgentId
	if agentID == "" {
		return &pb.ExplainQueryResponse{
			Status:       "error",
			ErrorMessage: "agent_id boş olamaz",
		}, fmt.Errorf("agent_id boş olamaz")
	}

	// Sorguyu kontrol et
	query := req.Query
	if query == "" {
		return &pb.ExplainQueryResponse{
			Status:       "error",
			ErrorMessage: "sorgu boş olamaz",
		}, fmt.Errorf("sorgu boş olamaz")
	}

	// Veritabanı adını kontrol et
	database := req.Database
	if database == "" {
		return &pb.ExplainQueryResponse{
			Status:       "error",
			ErrorMessage: "database boş olamaz",
		}, fmt.Errorf("database boş olamaz")
	}

	// Sorguyu MongoDB explain komutu olarak gönder
	// Yeni protokol formatı: MONGO_EXPLAIN|<database>|<query_json>
	explainCommand := fmt.Sprintf("MONGO_EXPLAIN|%s|%s", database, query)

	log.Printf("[DEBUG] MongoDB Explain protokol formatı: MONGO_EXPLAIN|<database>|<query_json>")
	log.Printf("[DEBUG] Hazırlanan sorgu komut uzunluğu: %d bytes", len(explainCommand))

	// Sorgunun ilk kısmını loglayalım, çok uzunsa sadece başını
	if len(query) > 500 {
		log.Printf("[DEBUG] Sorgu (ilk 500 karakter): %s...", query[:500])
	} else {
		log.Printf("[DEBUG] Sorgu: %s", query)
	}

	// Unique bir sorgu ID'si oluştur
	queryID := fmt.Sprintf("mongo_explain_%d", time.Now().UnixNano())

	// Uzun işlem için timeout süresini artır (60 saniye)
	longCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// Agent'a sorguyu gönder ve cevabı al
	log.Printf("[DEBUG] MongoDB sorgusu agent'a gönderiliyor: %s", agentID)
	// Burada database parametresini boş geçiyoruz çünkü zaten explainCommand içinde belirttik
	result, err := s.SendQuery(longCtx, agentID, queryID, explainCommand, "")
	if err != nil {
		log.Printf("[ERROR] MongoDB sorgu planı alınırken hata: %v", err)
		errMsg := err.Error()
		if err == context.DeadlineExceeded {
			errMsg = "sorgu zaman aşımına uğradı"
		}
		return &pb.ExplainQueryResponse{
			Status:       "error",
			ErrorMessage: fmt.Sprintf("MongoDB sorgu planı alınırken bir hata oluştu: %s", errMsg),
		}, err
	}

	// Sorgu sonucunu döndür
	if result.Result != nil {
		log.Printf("[DEBUG] MongoDB sorgu planı alındı, type_url: %s", result.Result.TypeUrl)

		// Agent'tan gelen ham veriyi direkt kullan
		var rawResult string

		// TypeUrl'e göre işlem yap - direkt ham veriyi çıkarmaya çalış
		if result.Result.TypeUrl == "type.googleapis.com/google.protobuf.Value" {
			// Value tipinde olanlar için direkt string değerini kullan
			rawResult = string(result.Result.Value)
			log.Printf("[DEBUG] Value tipi veri direkt string olarak kullanıldı, uzunluk: %d", len(rawResult))
		} else {
			// Struct veya diğer tipler için unmarshalling yapılmalı
			var resultStruct structpb.Struct
			if err := result.Result.UnmarshalTo(&resultStruct); err != nil {
				log.Printf("[ERROR] MongoDB sonucu ayrıştırılamadı: %v", err)
				return &pb.ExplainQueryResponse{
					Status:       "error",
					ErrorMessage: fmt.Sprintf("MongoDB sorgu planı ayrıştırılamadı: %v", err),
				}, err
			}

			// Struct'ı JSON olarak serialize et
			resultMap := resultStruct.AsMap()
			resultBytes, err := json.Marshal(resultMap)
			if err != nil {
				log.Printf("[ERROR] MongoDB JSON serileştirilirken hata: %v", err)
				return &pb.ExplainQueryResponse{
					Status:       "error",
					ErrorMessage: fmt.Sprintf("MongoDB JSON serileştirme hatası: %v", err),
				}, err
			}

			rawResult = string(resultBytes)
		}

		// Yanıtı döndür
		return &pb.ExplainQueryResponse{
			Status: "success",
			Plan:   rawResult,
		}, nil
	}

	// Sonuç boş ise hata döndür
	log.Printf("[ERROR] MongoDB sorgu planı boş sonuç döndü")
	return &pb.ExplainQueryResponse{
		Status:       "error",
		ErrorMessage: "MongoDB sorgu planı alınamadı: Boş sonuç",
	}, fmt.Errorf("MongoDB sorgu planı alınamadı: Boş sonuç")
}

// SendMSSQLInfo, agent'dan gelen MSSQL bilgilerini işler
func (s *Server) SendMSSQLInfo(ctx context.Context, req *pb.MSSQLInfoRequest) (*pb.MSSQLInfoResponse, error) {
	log.Println("SendMSSQLInfo metodu çağrıldı")

	// Gelen MSSQL bilgilerini logla
	mssqlInfo := req.MssqlInfo
	log.Printf("MSSQL bilgileri alındı: %+v", mssqlInfo)

	// Daha detaylı loglama
	log.Printf("Cluster: %s, IP: %s, Hostname: %s", mssqlInfo.ClusterName, mssqlInfo.Ip, mssqlInfo.Hostname)
	log.Printf("Node Durumu: %s, MSSQL Sürümü: %s, Konum: %s", mssqlInfo.NodeStatus, mssqlInfo.Version, mssqlInfo.Location)
	log.Printf("MSSQL Servis Durumu: %s, Instance: %s", mssqlInfo.Status, mssqlInfo.Instance)
	log.Printf("Boş Disk: %s, FD Yüzdesi: %d%%", mssqlInfo.FreeDisk, mssqlInfo.FdPercent)
	log.Printf("HA Enabled: %t, HA Role: %s, Edition: %s", mssqlInfo.IsHaEnabled, mssqlInfo.HaRole, mssqlInfo.Edition)

	// AlwaysOn bilgilerini logla
	if mssqlInfo.IsHaEnabled && mssqlInfo.AlwaysOnMetrics != nil {
		alwaysOn := mssqlInfo.AlwaysOnMetrics
		log.Printf("AlwaysOn Cluster: %s, Health: %s, Operational: %s",
			alwaysOn.ClusterName, alwaysOn.HealthState, alwaysOn.OperationalState)
		log.Printf("Primary Replica: %s, Local Role: %s, Sync Mode: %s",
			alwaysOn.PrimaryReplica, alwaysOn.LocalRole, alwaysOn.SynchronizationMode)
		log.Printf("Replication Lag: %d ms, Log Send Queue: %d KB, Redo Queue: %d KB",
			alwaysOn.ReplicationLagMs, alwaysOn.LogSendQueueKb, alwaysOn.RedoQueueKb)

		if len(alwaysOn.Replicas) > 0 {
			log.Printf("AlwaysOn Replicas count: %d", len(alwaysOn.Replicas))
			for i, replica := range alwaysOn.Replicas {
				log.Printf("Replica %d: %s (Role: %s, Connection: %s)",
					i+1, replica.ReplicaName, replica.Role, replica.ConnectionState)
			}
		}

		if len(alwaysOn.Databases) > 0 {
			log.Printf("AlwaysOn Databases count: %d", len(alwaysOn.Databases))
			for i, db := range alwaysOn.Databases {
				log.Printf("Database %d: %s (Sync State: %s, Replica: %s)",
					i+1, db.DatabaseName, db.SynchronizationState, db.ReplicaName)
			}
		}

		if len(alwaysOn.Listeners) > 0 {
			log.Printf("AlwaysOn Listeners count: %d", len(alwaysOn.Listeners))
			for i, listener := range alwaysOn.Listeners {
				log.Printf("Listener %d: %s (Port: %d, State: %s)",
					i+1, listener.ListenerName, listener.Port, listener.ListenerState)
			}
		}
	} else if mssqlInfo.IsHaEnabled {
		log.Printf("HA enabled but AlwaysOn metrics not available")
	}

	// Veritabanına kaydetme işlemi
	err := s.saveMSSQLInfoToDatabase(ctx, mssqlInfo)
	if err != nil {
		log.Printf("MSSQL bilgileri veritabanına kaydedilemedi: %v", err)
		return &pb.MSSQLInfoResponse{
			Status: "error",
		}, nil
	}

	log.Printf("MSSQL bilgileri başarıyla işlendi ve kaydedildi")

	return &pb.MSSQLInfoResponse{
		Status: "success",
	}, nil
}

// MSSQL bilgilerini veritabanına kaydetmek için yardımcı fonksiyon
func (s *Server) saveMSSQLInfoToDatabase(ctx context.Context, mssqlInfo *pb.MSSQLInfo) error {
	// Önce mevcut kaydı kontrol et
	var existingData []byte
	var id int

	checkQuery := `
		SELECT id, jsondata FROM public.mssql_data 
		WHERE clustername = $1 
		ORDER BY id DESC LIMIT 1
	`

	err := s.db.QueryRowContext(ctx, checkQuery, mssqlInfo.ClusterName).Scan(&id, &existingData)

	// Yeni node verisi
	mssqlData := map[string]interface{}{
		"ClusterName": mssqlInfo.ClusterName,
		"Location":    mssqlInfo.Location,
		"FDPercent":   mssqlInfo.FdPercent,
		"FreeDisk":    mssqlInfo.FreeDisk,
		"Hostname":    mssqlInfo.Hostname,
		"IP":          mssqlInfo.Ip,
		"NodeStatus":  mssqlInfo.NodeStatus,
		"Status":      mssqlInfo.Status,
		"Version":     mssqlInfo.Version,
		"Instance":    mssqlInfo.Instance,
		"Port":        mssqlInfo.Port,
		"TotalVCPU":   mssqlInfo.TotalVcpu,
		"TotalMemory": mssqlInfo.TotalMemory,
		"ConfigPath":  mssqlInfo.ConfigPath,
		"Database":    mssqlInfo.Database,
		"IsHAEnabled": mssqlInfo.IsHaEnabled,
		"HARole":      mssqlInfo.HaRole,
		"Edition":     mssqlInfo.Edition,
	}

	// AlwaysOn bilgilerini ekle (eğer mevcutsa)
	if mssqlInfo.IsHaEnabled && mssqlInfo.AlwaysOnMetrics != nil {
		alwaysOnData := map[string]interface{}{
			"ClusterName":         mssqlInfo.AlwaysOnMetrics.ClusterName,
			"HealthState":         mssqlInfo.AlwaysOnMetrics.HealthState,
			"OperationalState":    mssqlInfo.AlwaysOnMetrics.OperationalState,
			"SynchronizationMode": mssqlInfo.AlwaysOnMetrics.SynchronizationMode,
			"FailoverMode":        mssqlInfo.AlwaysOnMetrics.FailoverMode,
			"PrimaryReplica":      mssqlInfo.AlwaysOnMetrics.PrimaryReplica,
			"LocalRole":           mssqlInfo.AlwaysOnMetrics.LocalRole,
			"LastFailoverTime":    mssqlInfo.AlwaysOnMetrics.LastFailoverTime,
			"ReplicationLagMs":    mssqlInfo.AlwaysOnMetrics.ReplicationLagMs,
			"LogSendQueueKb":      mssqlInfo.AlwaysOnMetrics.LogSendQueueKb,
			"RedoQueueKb":         mssqlInfo.AlwaysOnMetrics.RedoQueueKb,
		}

		// Replica bilgilerini ekle
		if len(mssqlInfo.AlwaysOnMetrics.Replicas) > 0 {
			replicas := make([]map[string]interface{}, len(mssqlInfo.AlwaysOnMetrics.Replicas))
			for i, replica := range mssqlInfo.AlwaysOnMetrics.Replicas {
				replicas[i] = map[string]interface{}{
					"ReplicaName":         replica.ReplicaName,
					"Role":                replica.Role,
					"ConnectionState":     replica.ConnectionState,
					"SynchronizationMode": replica.SynchronizationMode,
					"FailoverMode":        replica.FailoverMode,
					"AvailabilityMode":    replica.AvailabilityMode,
					"JoinState":           replica.JoinState,
					"ConnectedState":      replica.ConnectedState,
					"SuspendReason":       replica.SuspendReason,
				}
			}
			alwaysOnData["Replicas"] = replicas
		}

		// Database bilgilerini ekle
		if len(mssqlInfo.AlwaysOnMetrics.Databases) > 0 {
			databases := make([]map[string]interface{}, len(mssqlInfo.AlwaysOnMetrics.Databases))
			for i, db := range mssqlInfo.AlwaysOnMetrics.Databases {
				databases[i] = map[string]interface{}{
					"DatabaseName":         db.DatabaseName,
					"ReplicaName":          db.ReplicaName,
					"SynchronizationState": db.SynchronizationState,
					"SuspendReason":        db.SuspendReason,
					"LastSentTime":         db.LastSentTime,
					"LastReceivedTime":     db.LastReceivedTime,
					"LastHardenedTime":     db.LastHardenedTime,
					"LastRedoneTime":       db.LastRedoneTime,
					"LogSendQueueKb":       db.LogSendQueueKb,
					"LogSendRateKbPerSec":  db.LogSendRateKbPerSec,
					"RedoQueueKb":          db.RedoQueueKb,
					"RedoRateKbPerSec":     db.RedoRateKbPerSec,
					"EndOfLogLsn":          db.EndOfLogLsn,
					"RecoveryLsn":          db.RecoveryLsn,
					"TruncationLsn":        db.TruncationLsn,
					"LastCommitLsn":        db.LastCommitLsn,
					"LastCommitTime":       db.LastCommitTime,
				}
			}
			alwaysOnData["Databases"] = databases
		}

		// Listener bilgilerini ekle
		if len(mssqlInfo.AlwaysOnMetrics.Listeners) > 0 {
			listeners := make([]map[string]interface{}, len(mssqlInfo.AlwaysOnMetrics.Listeners))
			for i, listener := range mssqlInfo.AlwaysOnMetrics.Listeners {
				listeners[i] = map[string]interface{}{
					"ListenerName":  listener.ListenerName,
					"IpAddresses":   listener.IpAddresses,
					"Port":          listener.Port,
					"SubnetMask":    listener.SubnetMask,
					"ListenerState": listener.ListenerState,
					"DnsName":       listener.DnsName,
				}
			}
			alwaysOnData["Listeners"] = listeners
		}

		mssqlData["AlwaysOnMetrics"] = alwaysOnData
		log.Printf("AlwaysOn metrics veritabanına kaydedildi: %s", mssqlInfo.Hostname)
	}

	var jsonData []byte

	// Hata kontrolünü düzgün yap
	if err == nil {
		// Mevcut kayıt var, güncelle
		log.Printf("MSSQL cluster için mevcut kayıt bulundu, güncelleniyor: %s (ID: %d)", mssqlInfo.ClusterName, id)

		var existingJSON map[string][]interface{}
		if err := json.Unmarshal(existingData, &existingJSON); err != nil {
			log.Printf("Mevcut JSON ayrıştırma hatası: %v", err)
			return err
		}

		// Cluster array'ini al
		clusterData, ok := existingJSON[mssqlInfo.ClusterName]
		if !ok {
			// Eğer cluster verisi yoksa yeni oluştur
			clusterData = []interface{}{}
		}

		// Node'u bul ve güncelle
		nodeFound := false
		nodeChanged := false
		for i, node := range clusterData {
			nodeMap, ok := node.(map[string]interface{})
			if !ok {
				continue
			}

			// Hostname ve IP ile node eşleşmesi kontrol et
			if nodeMap["Hostname"] == mssqlInfo.Hostname && nodeMap["IP"] == mssqlInfo.Ip {
				// Sadece değişen alanları güncelle
				nodeFound = true

				// Değişiklikleri takip et
				for key, newValue := range mssqlData {
					currentValue, exists := nodeMap[key]
					var hasChanged bool

					if !exists {
						// Değer mevcut değil, yeni alan ekleniyor
						hasChanged = true
						log.Printf("MSSQL node'da yeni alan eklendi: %s, %s", mssqlInfo.Hostname, key)
					} else {
						// Mevcut değer ile yeni değeri karşılaştır
						// Numeric değerler için özel karşılaştırma yap
						hasChanged = !compareValues(currentValue, newValue)
						if hasChanged {
							if key == "AlwaysOnMetrics" {
								log.Printf("MSSQL node'da AlwaysOn metrics güncellendi: %s", mssqlInfo.Hostname)
							} else {
								log.Printf("MSSQL node'da değişiklik tespit edildi: %s, %s: %v -> %v",
									mssqlInfo.Hostname, key, currentValue, newValue)
							}
						}
					}

					if hasChanged {
						nodeMap[key] = newValue
						// HARole, Status, AlwaysOnMetrics gibi önemli bir değişiklik varsa işaretle
						if key == "HARole" || key == "Status" || key == "NodeStatus" || key == "FreeDisk" || key == "AlwaysOnMetrics" {
							nodeChanged = true
						}
					}
				}

				clusterData[i] = nodeMap
				break
			}
		}

		// Eğer node bulunamadıysa yeni ekle
		if !nodeFound {
			clusterData = append(clusterData, mssqlData)
			nodeChanged = true
			log.Printf("Yeni MSSQL node eklendi: %s", mssqlInfo.Hostname)
		}

		// Eğer önemli bir değişiklik yoksa veritabanını güncelleme
		if !nodeChanged {
			log.Printf("MSSQL node'da önemli bir değişiklik yok, güncelleme yapılmadı: %s", mssqlInfo.Hostname)
			return nil
		}

		existingJSON[mssqlInfo.ClusterName] = clusterData

		// JSON'ı güncelle
		jsonData, err = json.Marshal(existingJSON)
		if err != nil {
			log.Printf("JSON dönüştürme hatası: %v", err)
			return err
		}

		// Veritabanını güncelle
		updateQuery := `
			UPDATE public.mssql_data 
			SET jsondata = $1, updated_at = CURRENT_TIMESTAMP
			WHERE id = $2
		`

		_, err = s.db.ExecContext(ctx, updateQuery, jsonData, id)
		if err != nil {
			log.Printf("Veritabanı güncelleme hatası: %v", err)
			return err
		}

		log.Printf("MSSQL node bilgileri başarıyla güncellendi (önemli değişiklik nedeniyle)")
	} else if err == sql.ErrNoRows {
		// Kayıt bulunamadı, yeni kayıt oluştur
		log.Printf("MSSQL cluster için kayıt bulunamadı, yeni kayıt oluşturuluyor: %s", mssqlInfo.ClusterName)

		outerJSON := map[string][]interface{}{
			mssqlInfo.ClusterName: {mssqlData},
		}

		jsonData, err = json.Marshal(outerJSON)
		if err != nil {
			log.Printf("JSON dönüştürme hatası: %v", err)
			return err
		}

		insertQuery := `
			INSERT INTO public.mssql_data (
				jsondata, clustername, created_at, updated_at
			) VALUES ($1, $2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			ON CONFLICT (clustername) DO UPDATE SET
				jsondata = EXCLUDED.jsondata,
				updated_at = CURRENT_TIMESTAMP
		`

		_, err = s.db.ExecContext(ctx, insertQuery, jsonData, mssqlInfo.ClusterName)
		if err != nil {
			log.Printf("Veritabanı ekleme hatası: %v", err)
			return err
		}

		log.Printf("MSSQL node bilgileri başarıyla veritabanına kaydedildi (yeni kayıt)")
	} else {
		// Başka bir veritabanı hatası oluştu
		log.Printf("MSSQL cluster kayıt kontrolü sırasında hata: %v", err)
		return fmt.Errorf("veritabanı kontrol hatası: %v", err)
	}

	return nil
}

// GetStatusMSSQL, MSSQL veritabanından durum bilgilerini çeker
func (s *Server) GetStatusMSSQL(ctx context.Context, _ *structpb.Struct) (*structpb.Value, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT json_agg(sub.jsondata) FROM (SELECT jsondata FROM mssql_data ORDER BY id) AS sub")
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

// GetMSSQLBestPracticesAnalysis, MSSQL best practice analizi gerçekleştirir
func (s *Server) GetMSSQLBestPracticesAnalysis(ctx context.Context, agentID, database string) (*pb.BestPracticesAnalysisResponse, error) {
	log.Printf("[INFO] GetMSSQLBestPracticesAnalysis çağrıldı - Agent ID: %s, Database: %s", agentID, database)

	// Agent ID'yi kontrol et
	if agentID == "" {
		return nil, fmt.Errorf("agent_id boş olamaz")
	}

	// BestPracticesAnalysisRequest oluştur
	req := &pb.BestPracticesAnalysisRequest{
		AgentId:      agentID,
		DatabaseName: database,
	}

	// RPC çağrısını yap
	return s.GetBestPracticesAnalysis(ctx, req)
}

// GetBestPracticesAnalysis, MSSQL best practice analizi için gRPC çağrısını yapar
func (s *Server) GetBestPracticesAnalysis(ctx context.Context, req *pb.BestPracticesAnalysisRequest) (*pb.BestPracticesAnalysisResponse, error) {
	agentID := req.AgentId

	// Agent'ın bağlı olup olmadığını kontrol et
	s.mu.RLock()
	agent, agentExists := s.agents[agentID]
	s.mu.RUnlock()

	if !agentExists || agent == nil {
		return nil, fmt.Errorf("agent bulunamadı veya bağlantı kapalı: %s", agentID)
	}

	// Unique bir sorgu ID'si oluştur
	queryID := fmt.Sprintf("mssql_bpa_%d", time.Now().UnixNano())

	// MSSQL_BPA formatında komut oluştur (database parametresi opsiyonel)
	command := "MSSQL_BPA"
	if req.DatabaseName != "" {
		command = fmt.Sprintf("MSSQL_BPA|%s", req.DatabaseName)
	}
	if req.ServerName != "" {
		command = fmt.Sprintf("%s|%s", command, req.ServerName)
	}

	log.Printf("[DEBUG] MSSQL Best Practices Analysis komut: %s", command)

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
		log.Printf("[ERROR] MSSQL Best Practices Analysis sorgusu gönderilemedi: %v", err)
		return nil, fmt.Errorf("sorgu gönderilemedi: %v", err)
	}

	log.Printf("[INFO] MSSQL Best Practices Analysis sorgusu gönderildi - Agent: %s, QueryID: %s", agentID, queryID)

	// Cevabı bekle
	select {
	case result := <-resultChan:
		// Sonuç geldi
		if result == nil {
			log.Printf("[ERROR] Null sorgu sonucu alındı")
			return nil, fmt.Errorf("null sorgu sonucu alındı")
		}

		log.Printf("[DEBUG] Agent'tan yanıt alındı - QueryID: %s, TypeUrl: %s", queryID, result.Result.TypeUrl)

		// Sonucu BestPracticesAnalysisResponse'a dönüştür
		response := &pb.BestPracticesAnalysisResponse{
			Status:            "success",
			AnalysisId:        queryID,
			AnalysisTimestamp: time.Now().Format(time.RFC3339),
		}

		// Any tipini çözümle
		if result.Result.TypeUrl == "type.googleapis.com/google.protobuf.Value" {
			// Value tipinde ise, doğrudan JSON string olarak kullan
			response.AnalysisResults = result.Result.Value
			log.Printf("[DEBUG] Best Practices Analysis sonucu alındı (JSON string, %d bytes)", len(response.AnalysisResults))
		} else {
			// Struct olarak çözümle
			var resultStruct structpb.Struct
			if err := result.Result.UnmarshalTo(&resultStruct); err != nil {
				log.Printf("[ERROR] Struct çözümleme hatası: %v", err)
				return nil, fmt.Errorf("sonuç çözümleme hatası: %v", err)
			}

			// Struct'tan JSON string oluştur
			resultBytes, err := json.Marshal(resultStruct.AsMap())
			if err != nil {
				log.Printf("[ERROR] JSON dönüştürme hatası: %v", err)
				return nil, fmt.Errorf("JSON dönüştürme hatası: %v", err)
			}

			response.AnalysisResults = resultBytes
			log.Printf("[DEBUG] Best Practices Analysis sonucu alındı (Struct->JSON, %d bytes)", len(response.AnalysisResults))
		}

		return response, nil

	case <-ctx.Done():
		// Context iptal edildi veya zaman aşımına uğradı
		log.Printf("[ERROR] Best Practices Analysis sonucu beklerken timeout/iptal: %v", ctx.Err())
		return nil, ctx.Err()
	}
}

// ReportProcessLogs, agent'lardan gelen işlem loglarını işler
func (s *Server) ReportProcessLogs(ctx context.Context, req *pb.ProcessLogRequest) (*pb.ProcessLogResponse, error) {
	log.Printf("ReportProcessLogs metodu çağrıldı - Agent ID: %s, Process ID: %s, Process Type: %s",
		req.LogUpdate.AgentId, req.LogUpdate.ProcessId, req.LogUpdate.ProcessType)

	// Gelen log mesajlarını logla
	for _, msg := range req.LogUpdate.LogMessages {
		log.Printf("[Process Log] [%s] [%s] %s", req.LogUpdate.ProcessId, req.LogUpdate.Status, msg)
	}

	// Veritabanına kaydet
	err := s.saveProcessLogs(ctx, req.LogUpdate)
	if err != nil {
		log.Printf("Process logları veritabanına kaydedilemedi: %v", err)
		return &pb.ProcessLogResponse{
			Status:  "error",
			Message: fmt.Sprintf("Veritabanı hatası: %v", err),
		}, nil
	}

	return &pb.ProcessLogResponse{
		Status:  "ok",
		Message: "Process logları başarıyla alındı",
	}, nil
}

// saveProcessLogs, işlem loglarını veritabanına kaydeder
func (s *Server) saveProcessLogs(ctx context.Context, logUpdate *pb.ProcessLogUpdate) error {
	// Veritabanı bağlantısını kontrol et
	if err := s.checkDatabaseConnection(); err != nil {
		return fmt.Errorf("veritabanı bağlantı hatası: %v", err)
	}

	// Metadata'yı JSON formatına dönüştür
	metadataJSON, err := json.Marshal(logUpdate.Metadata)
	if err != nil {
		return fmt.Errorf("metadata JSON dönüştürme hatası: %v", err)
	}

	// Log mesajlarını JSON formatına dönüştür
	logMessagesJSON, err := json.Marshal(logUpdate.LogMessages)
	if err != nil {
		return fmt.Errorf("log mesajları JSON dönüştürme hatası: %v", err)
	}

	// İşlemin var olup olmadığını kontrol et
	var processExists bool
	err = s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM process_logs 
			WHERE process_id = $1 AND agent_id = $2
		)
	`, logUpdate.ProcessId, logUpdate.AgentId).Scan(&processExists)

	if err != nil {
		return fmt.Errorf("işlem kaydı kontrolü sırasında hata: %v", err)
	}

	if !processExists {
		// İşlem ilk kez kaydediliyor
		log.Printf("Yeni işlem kaydı oluşturuluyor: %s", logUpdate.ProcessId)

		insertQuery := `
			INSERT INTO process_logs (
				process_id,
				agent_id,
				process_type,
				status,
				log_messages,
				elapsed_time_s,
				metadata,
				created_at,
				updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
		`

		_, err = s.db.ExecContext(ctx, insertQuery,
			logUpdate.ProcessId,
			logUpdate.AgentId,
			logUpdate.ProcessType,
			logUpdate.Status,
			logMessagesJSON,
			logUpdate.ElapsedTimeS,
			metadataJSON,
			time.Now(),
		)

		if err != nil {
			return fmt.Errorf("yeni işlem kaydı eklenirken hata: %v", err)
		}
	} else {
		// Mevcut işlem güncelleniyor
		log.Printf("Mevcut işlem kaydı güncelleniyor: %s", logUpdate.ProcessId)

		// Mevcut log mesajlarını al
		var existingLogs []byte
		err = s.db.QueryRowContext(ctx, `
			SELECT log_messages FROM process_logs 
			WHERE process_id = $1 AND agent_id = $2
		`, logUpdate.ProcessId, logUpdate.AgentId).Scan(&existingLogs)

		if err != nil {
			return fmt.Errorf("mevcut log mesajları alınırken hata: %v", err)
		}

		// Mevcut log mesajlarını çözümle
		var existingLogMessages []string
		if err := json.Unmarshal(existingLogs, &existingLogMessages); err != nil {
			return fmt.Errorf("mevcut log mesajları ayrıştırılırken hata: %v", err)
		}

		// Yeni log mesajlarını ekle
		combinedLogMessages := append(existingLogMessages, logUpdate.LogMessages...)

		// Birleştirilmiş log mesajlarını JSON'a dönüştür
		combinedLogsJSON, err := json.Marshal(combinedLogMessages)
		if err != nil {
			return fmt.Errorf("birleştirilmiş log mesajları JSON dönüştürme hatası: %v", err)
		}

		// Güncelleme sorgusunu çalıştır
		updateQuery := `
			UPDATE process_logs SET 
				status = $1,
				log_messages = $2,
				elapsed_time_s = $3,
				metadata = $4,
				updated_at = $5
			WHERE process_id = $6 AND agent_id = $7
		`

		_, err = s.db.ExecContext(ctx, updateQuery,
			logUpdate.Status,
			combinedLogsJSON,
			logUpdate.ElapsedTimeS,
			metadataJSON,
			time.Now(),
			logUpdate.ProcessId,
			logUpdate.AgentId,
		)

		if err != nil {
			return fmt.Errorf("işlem kaydı güncellenirken hata: %v", err)
		}
	}

	// Eğer işlem tamamlandıysa veya başarısız olduysa, job durumunu da güncelle
	if logUpdate.Status == "completed" || logUpdate.Status == "failed" {
		log.Printf("Process %s tamamlandı, job durumu güncelleniyor. Status: %s", logUpdate.ProcessId, logUpdate.Status)

		// Process ID ile job'ı bul (job_id olarak process_id kullanılıyor)
		s.jobMu.Lock()
		job, exists := s.jobs[logUpdate.ProcessId]

		if exists {
			// Job durumunu güncelle
			if logUpdate.Status == "completed" {
				job.Status = pb.JobStatus_JOB_STATUS_COMPLETED
				job.Result = "Job completed successfully by agent process logger"
			} else {
				job.Status = pb.JobStatus_JOB_STATUS_FAILED
				job.ErrorMessage = "Job failed as reported by agent process logger"
			}

			job.UpdatedAt = timestamppb.Now()
			s.jobs[logUpdate.ProcessId] = job
			s.jobMu.Unlock()

			// Veritabanında da job durumunu güncelle
			err = s.updateJobInDatabase(context.Background(), job)
			if err != nil {
				log.Printf("Job durumu veritabanında güncellenirken hata: %v", err)
			} else {
				log.Printf("Job durumu başarıyla güncellendi: %s -> %s", logUpdate.ProcessId, job.Status.String())
			}
		} else {
			s.jobMu.Unlock()
			log.Printf("Process ID'ye karşılık gelen job bulunamadı: %s", logUpdate.ProcessId)
		}
	}

	log.Printf("Process logları başarıyla kaydedildi - Process ID: %s", logUpdate.ProcessId)
	return nil
}

// GetProcessStatus, belirli bir işlemin durumunu sorgular
func (s *Server) GetProcessStatus(ctx context.Context, req *pb.ProcessStatusRequest) (*pb.ProcessStatusResponse, error) {
	log.Printf("GetProcessStatus metodu çağrıldı - Agent ID: %s, Process ID: %s", req.AgentId, req.ProcessId)

	// Veritabanı bağlantısını kontrol et
	if err := s.checkDatabaseConnection(); err != nil {
		return nil, fmt.Errorf("veritabanı bağlantı hatası: %v", err)
	}

	// SQL sorgusu hazırla
	var query string
	var args []interface{}

	if req.AgentId != "" {
		// Agent ID verilmişse hem process_id hem agent_id ile sorgula
		query = `
			SELECT process_id, agent_id, process_type, status, log_messages, elapsed_time_s, metadata, created_at, updated_at
			FROM process_logs
			WHERE process_id = $1 AND agent_id = $2
		`
		args = []interface{}{req.ProcessId, req.AgentId}
	} else {
		// Agent ID verilmemişse sadece process_id ile sorgula
		query = `
			SELECT process_id, agent_id, process_type, status, log_messages, elapsed_time_s, metadata, created_at, updated_at
			FROM process_logs
			WHERE process_id = $1
		`
		args = []interface{}{req.ProcessId}
	}

	log.Printf("Process sorgusu: %s, Parametreler: %v", query, args)

	var (
		processID       string
		agentID         string
		processType     string
		status          string
		logMessagesJSON []byte
		elapsedTimeS    float32
		metadataJSON    []byte
		createdAt       time.Time
		updatedAt       time.Time
	)

	// Sorguyu çalıştır
	err := s.db.QueryRowContext(ctx, query, args...).Scan(
		&processID,
		&agentID,
		&processType,
		&status,
		&logMessagesJSON,
		&elapsedTimeS,
		&metadataJSON,
		&createdAt,
		&updatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("Process bulunamadı: %s", req.ProcessId)
			return nil, fmt.Errorf("process bulunamadı: %s", req.ProcessId)
		}
		log.Printf("Process bilgileri alınırken hata: %v", err)
		return nil, fmt.Errorf("process bilgileri alınırken hata: %v", err)
	}

	// Log mesajlarını çözümle
	var logMessages []string
	if err := json.Unmarshal(logMessagesJSON, &logMessages); err != nil {
		log.Printf("Log mesajları ayrıştırılırken hata: %v", err)
		return nil, fmt.Errorf("log mesajları ayrıştırılırken hata: %v", err)
	}

	// Metadata'yı çözümle
	var metadata map[string]string
	if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
		log.Printf("Metadata ayrıştırılırken hata: %v", err)
		return nil, fmt.Errorf("metadata ayrıştırılırken hata: %v", err)
	}

	log.Printf("Process bilgileri başarıyla alındı - Process ID: %s, Status: %s, Log sayısı: %d",
		processID, status, len(logMessages))

	return &pb.ProcessStatusResponse{
		ProcessId:    processID,
		ProcessType:  processType,
		Status:       status,
		ElapsedTimeS: elapsedTimeS,
		LogMessages:  logMessages,
		CreatedAt:    createdAt.Format(time.RFC3339),
		UpdatedAt:    updatedAt.Format(time.RFC3339),
		Metadata:     metadata,
	}, nil
}

// GetRecentAlarms, dashboard için optimize edilmiş son alarmları getirir
func (s *Server) GetRecentAlarms(ctx context.Context, limit int, onlyUnacknowledged bool) ([]map[string]interface{}, error) {
	// Veritabanı bağlantısını kontrol et
	if err := s.checkDatabaseConnection(); err != nil {
		return nil, fmt.Errorf("veritabanı bağlantı hatası: %v", err)
	}

	// Varsayılan limit kontrolü (dashboard için max 50)
	if limit <= 0 || limit > 50 {
		limit = 20 // Dashboard için varsayılan 20 kayıt
	}

	// Optimize edilmiş sorgu - sadece gerekli alanlar ve index kullanımı
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
		FROM alarms`

	var queryParams []interface{}
	if onlyUnacknowledged {
		query += ` WHERE acknowledged = false`
	}

	query += ` ORDER BY created_at DESC LIMIT $1`
	queryParams = append(queryParams, limit)

	log.Printf("Recent alarms sorgusu - Limit: %d, Unacknowledged: %t", limit, onlyUnacknowledged)

	// Sorguyu çalıştır
	rows, err := s.db.QueryContext(ctx, query, queryParams...)
	if err != nil {
		return nil, fmt.Errorf("recent alarm verileri çekilemedi: %v", err)
	}
	defer rows.Close()

	// Sonuçları topla (hafif veri yapısı)
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

		// Hafif alarm objesi (dashboard için gerekli minimum alanlar)
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

func compareValues(a, b interface{}) bool {
	// Nil kontrolü
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Numeric değerler için tip dönüşümü yaparak karşılaştır
	aFloat, aIsNumeric := convertToFloat64(a)
	bFloat, bIsNumeric := convertToFloat64(b)

	if aIsNumeric && bIsNumeric {
		// Float karşılaştırması için tolerans kullan (floating point precision)
		return math.Abs(aFloat-bFloat) < 1e-9
	}

	// String karşılaştırması
	if aStr, ok := a.(string); ok {
		if bStr, ok := b.(string); ok {
			return aStr == bStr
		}
		return false
	}

	// Boolean karşılaştırması
	if aBool, ok := a.(bool); ok {
		if bBool, ok := b.(bool); ok {
			return aBool == bBool
		}
		return false
	}

	// Complex types (maps, slices) için reflect.DeepEqual kullan
	return reflect.DeepEqual(a, b)
}

// convertToFloat64, farklı numeric tiplerini float64'e dönüştürür
func convertToFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int32:
		return float64(val), true
	case int64:
		return float64(val), true
	case uint:
		return float64(val), true
	case uint32:
		return float64(val), true
	case uint64:
		return float64(val), true
	default:
		return 0, false
	}
}
