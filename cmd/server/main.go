package main

import (
	"log"
	"net"
	"net/http"

	"github.com/sefaphlvn/clustereye-test/internal/api"
	"github.com/sefaphlvn/clustereye-test/internal/config"
	"github.com/sefaphlvn/clustereye-test/internal/database"
	"github.com/sefaphlvn/clustereye-test/internal/server"
	pb "github.com/sefaphlvn/clustereye-test/pkg/agent"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
)

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

	// gRPC Server başlat
	listener, err := net.Listen("tcp", cfg.GRPC.Address)
	if err != nil {
		log.Fatal(err)
	}

	grpcServer := grpc.NewServer()
	serverInstance := server.NewServer(db)
	pb.RegisterAgentServiceServer(grpcServer, serverInstance)

	go func() {
		log.Printf("Cloud API gRPC server çalışıyor: %s", cfg.GRPC.Address)
		if err := grpcServer.Serve(listener); err != nil {
			log.Fatal(err)
		}
	}()

	// HTTP Gin API Server başlat
	router := gin.Default()
	
	// API handler'larını kaydet
	api.RegisterHandlers(router, serverInstance)

	// HTTP sunucusunu başlat
	log.Printf("HTTP API server çalışıyor: %s", cfg.HTTP.Address)
	if err := http.ListenAndServe(cfg.HTTP.Address, router); err != nil {
		log.Fatal(err)
	}
} 