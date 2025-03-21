.PHONY: all build-server build-agent run-server run-agent clean proto

# Değişkenler
SERVER_OUT := bin/server
AGENT_OUT := bin/agent
SERVER_PKG := ./cmd/server
AGENT_PKG := ./cmd/agent
PROTO_DIR := ./pkg/agent

# Tüm hedefler
all: build-server build-agent

# Protocol Buffer dosyalarını derle
proto:
	@echo "Protocol buffer dosyaları derleniyor..."
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		$(PROTO_DIR)/agent.proto

# Server build etme
build-server:
	@echo "ClusterEye server build ediliyor..."
	mkdir -p bin
	go build -o $(SERVER_OUT) $(SERVER_PKG)

# Agent build etme
build-agent:
	@echo "ClusterEye agent build ediliyor..."
	mkdir -p bin
	go build -o $(AGENT_OUT) $(AGENT_PKG)

# Server çalıştırma
run-server: build-server
	@echo "ClusterEye server başlatılıyor..."
	$(SERVER_OUT)

# Agent çalıştırma
run-agent: build-agent
	@echo "ClusterEye agent başlatılıyor..."
	$(AGENT_OUT)

# Temizlik
clean:
	@echo "Temizlik yapılıyor..."
	rm -rf bin/
	
# Bağımlılıkları güncelle
deps:
	@echo "Bağımlılıklar güncelleniyor..."
	go mod tidy

# Test çalıştırma
test:
	@echo "Testler çalıştırılıyor..."
	go test -v ./...

# Yardım
help:
	@echo "Kullanılabilir hedefler:"
	@echo "  all:          Server ve agent'ı build et"
	@echo "  build-server: Server'ı build et"
	@echo "  build-agent:  Agent'ı build et"
	@echo "  run-server:   Server'ı build et ve çalıştır"
	@echo "  run-agent:    Agent'ı build et ve çalıştır"
	@echo "  proto:        Protocol buffer dosyalarını derle"
	@echo "  clean:        Oluşturulan dosyaları temizle"
	@echo "  deps:         Bağımlılıkları güncelle"
	@echo "  test:         Testleri çalıştır"
	@echo "  help:         Bu yardım mesajını göster" 