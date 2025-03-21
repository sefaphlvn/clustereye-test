# ClusterEye Projesi

ClusterEye, uzak sistemlerde çalışan veritabanları ve servisleri izlemek ve yönetmek için geliştirilmiş bir araçtır. Proje, ClusterEye sunucusu ve agent bileşenlerinden oluşur.

## Bileşenler

### ClusterEye Sunucusu

ClusterEye sunucusu, tüm agent'ları yönetir, verileri toplar ve bir API arayüzü sunar. Temel özellikleri:

- Agent'ların bağlantılarını yönetmek
- Agent'lardan veri toplamak
- REST API üzerinden agent'lara erişim sunmak
- PostgreSQL veritabanıyla çalışmak

### ClusterEye Agent

ClusterEye agent, izlenen sistemlerde çalışan ve verileri toplayan bileşendir:

- Sunucuya gRPC üzerinden bağlanarak kimlik doğrulaması yapar
- Konfigürasyon dosyasından ayarları okur
- Yerel sistem veritabanlarını (PostgreSQL, MongoDB) izler
- Sunucudan komut alıp çalıştırabilir

## Proje Yapısı

```
clustereye/
├── cmd/                  # Uygulamaların ana giriş noktaları
│   ├── agent/           # Agent uygulaması
│   │   └── main.go
│   └── server/          # Server uygulaması
│       └── main.go
├── internal/             # Dışarıya açık olmayan paketler
│   ├── api/             # API handler'ları
│   ├── config/          # Konfigürasyon yönetimi
│   ├── database/        # Veritabanı işlemleri
│   └── server/          # Sunucu işlemleri
├── pkg/                  # Dışarıya açık paketler
│   └── agent/           # API tanımlamaları (proto)
├── Makefile              # Build ve çalıştırma komutları
├── go.mod                # Go modül tanımlamaları
└── README.md             # Proje dokümantasyonu
```

## Kurulum ve Çalıştırma

### Ön Gereksinimler

- Go 1.18 veya üzeri
- PostgreSQL veritabanı
- Protocol Buffer Compiler (protoc)

### Kurulum

1. Projeyi klonlayın:
```bash
git clone https://github.com/sefaphlvn/clustereye-test.git
cd clustereye-test
```

2. Bağımlılıkları yükleyin:
```bash
go mod download
```

3. Protokol tampon dosyalarını derleyin (isteğe bağlı, önceden derlenmiş):
```bash
make proto
```

### Derleme

Her iki bileşeni de derlemek için:
```bash
make all
```

veya ayrı ayrı:
```bash
make build-server  # Sunucu için
make build-agent   # Agent için
```

### Çalıştırma

Sunucuyu çalıştırmak için:
```bash
make run-server
```

Agent'ı çalıştırmak için:
```bash
make run-agent
```

## Konfigürasyon

### Sunucu Konfigürasyonu

Sunucu, varsayılan olarak `server.yml` veya `/etc/clustereye/server.yml` dosyasını kullanır:

```yaml
server:
  name: ClusterEye
grpc:
  address: :50051
http:
  address: :8080
database:
  host: localhost
  port: 5432
  user: postgres
  password: postgres
  dbname: clustereye
  sslmode: disable
```

### Agent Konfigürasyonu

Agent, varsayılan olarak `agent.yml` veya `/etc/clustereye/agent.yml` dosyasını kullanır:

```yaml
key: agent_key
name: Agent
grpc:
  server_address: localhost:50051
postgresql:
  user: postgres
  pass: postgres
  port: 5432
  cluster: postgres
mongo:
  user: mongo
  pass: mongo
  port: 27017
  replset: rs0
```

## API Kullanımı

### Tüm Agent'ları Listeleme

```
GET /api/v1/agents
```

### Agent'a Sorgu Gönderme

```
POST /api/v1/agents/:agent_id/query
Content-Type: application/json

{
  "query_id": "unique-query-id",
  "command": "SELECT version();"
}
```

### PostgreSQL Durum Bilgilerini Alma

```
GET /api/v1/status/postgres
```

## Lisans

Bu proje [MIT Lisansı](LICENSE) altında lisanslanmıştır.