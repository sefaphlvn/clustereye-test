package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	pb "github.com/sefaphlvn/clustereye-test/pkg/agent"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

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
}

// NewServer, yeni bir sunucu nesnesi oluşturur
func NewServer(db *sql.DB) *Server {
	return &Server{
		agents:      make(map[string]*AgentConnection),
		queryResult: make(map[string]*QueryResponse),
		db:          db,
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
			currentAgentID = agentInfo.AgentId

			s.mu.Lock()
			s.agents[currentAgentID] = &AgentConnection{
				Stream: stream,
				Info:   agentInfo,
			}
			s.mu.Unlock()

			log.Printf("Yeni Agent bağlandı: %+v", agentInfo)

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

// SendQuery, belirli bir agent'a sorgu gönderir ve cevabı bekler
func (s *Server) SendQuery(ctx context.Context, agentID, queryID, command string) (*pb.QueryResult, error) {
	s.mu.RLock()
	agentConn, ok := s.agents[agentID]
	s.mu.RUnlock()

	if !ok {
		return nil, gin.Error{Err: http.ErrNoLocation, Type: gin.ErrorTypePublic}
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
		Query: &pb.Query{
			QueryId: queryID,
			Command: command,
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
		return nil, errors.New("sorgu zaman aşımına uğradı")
	}
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