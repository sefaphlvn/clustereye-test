package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	pb "github.com/sefaphlvn/clustereye-test/pkg/agent"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"
)

type AgentConnection struct {
	stream pb.AgentService_ConnectServer
	info   *pb.AgentInfo
}

type QueryResponse struct {
	Result     string
	ResultChan chan *pb.QueryResult
}

type Server struct {
	pb.UnimplementedAgentServiceServer
	mu          sync.RWMutex
	agents      map[string]*AgentConnection
	queryMu     sync.RWMutex
	queryResult map[string]*QueryResponse // query_id -> QueryResponse
}

func NewServer() *Server {
	return &Server{
		agents:      make(map[string]*AgentConnection),
		queryResult: make(map[string]*QueryResponse),
	}
}

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
				stream: stream,
				info:   agentInfo,
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
	err := agentConn.stream.Send(&pb.ServerMessage{
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

func main() {
	// gRPC Server başlat
	listener, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatal(err)
	}

	grpcServer := grpc.NewServer()
	server := NewServer()
	pb.RegisterAgentServiceServer(grpcServer, server)

	go func() {
		log.Println("Cloud API gRPC server çalışıyor: :50051")
		if err := grpcServer.Serve(listener); err != nil {
			log.Fatal(err)
		}
	}()

	// HTTP Gin API Server başlat
	router := gin.Default()

	// Agent'a sorgu göndermek için endpoint (query param ile)
	router.POST("/api/send-query", func(c *gin.Context) {
		agentID := c.Query("agent_id")

		if agentID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "agent_id query param gerekli"})
			return
		}

		var req struct {
			QueryID string `json:"query_id"`
			Command string `json:"command"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz JSON verisi"})
			return
		}

		// Context oluştur (request'in iptal edilmesi durumunda kullanılacak)
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		// Sorguyu gönder ve cevabı bekle
		result, err := server.SendQuery(ctx, agentID, req.QueryID, req.Command)
		if err != nil {
			if err == context.DeadlineExceeded {
				c.JSON(http.StatusGatewayTimeout, gin.H{"error": "Sorgu zaman aşımına uğradı"})
			} else {
				c.JSON(http.StatusNotFound, gin.H{"error": "Agent bulunamadı veya bağlantı kapalı"})
			}
			return
		}

		// Any tipindeki sonucu çözümle
		var structResult structpb.Struct
		if err := result.Result.UnmarshalTo(&structResult); err != nil {
			// Hata durumunda orijinal veriyi gönder
			c.JSON(http.StatusOK, gin.H{
				"status":    "Sorgu tamamlandı",
				"agent_id":  agentID,
				"query_id":  result.QueryId,
				"result":    result.Result,
			})
			return
		}

		// Struct'tan Go Map'ine dönüştür
		resultMap := structResult.AsMap()

		c.JSON(http.StatusOK, gin.H{
			"status":    "Sorgu tamamlandı",
			"agent_id":  agentID,
			"query_id":  result.QueryId,
			"result":    resultMap,
		})
	})

	log.Println("HTTP Gin server çalışıyor: :8080")
	router.Run(":8080")
}