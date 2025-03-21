package main

import (
	"context"
	"log"
	"net"
	"os"
	"time"

	pb "github.com/sefaphlvn/clustereye-test/pkg/agent"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"
)

func main() {
	conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	client := pb.NewAgentServiceClient(conn)
	stream, err := client.Connect(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	hostname, _ := os.Hostname()
	ip := getLocalIP()

	// Agent bilgilerini gönder
	agentInfo := &pb.AgentMessage{
		Payload: &pb.AgentMessage_AgentInfo{
			AgentInfo: &pb.AgentInfo{
				AgentId:  "agent123",
				Hostname: hostname,
				Ip:       ip,
			},
		},
	}

	if err := stream.Send(agentInfo); err != nil {
		log.Fatal(err)
	}

	go func() {
		for {
			in, err := stream.Recv()
			if err != nil {
				log.Fatalf("Cloud API bağlantısı kapandı: %v", err)
			}

			if query := in.GetQuery(); query != nil {
				log.Printf("Yeni sorgu geldi: %s", query.Command)

				// Map'i structpb'ye dönüştür
				resultMap, _ := structpb.NewStruct(map[string]interface{}{
					"test": "test",
				})
				
				// structpb'yi Any'e dönüştür
				anyResult, _ := anypb.New(resultMap)

				// Sorguyu işle ve sonucu gönder
				result := &pb.AgentMessage{
					Payload: &pb.AgentMessage_QueryResult{
						QueryResult: &pb.QueryResult{
							QueryId: query.QueryId,
							Result:  anyResult,
						},
					},
				}

				if err := stream.Send(result); err != nil {
					log.Fatal(err)
				}
			}
		}
	}()

	// Agent bağlantısını canlı tutmak için basit bir döngü
	for {
		time.Sleep(time.Minute)
	}
}

// Yerel IP'yi almak için basit fonksiyon
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
