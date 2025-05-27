# ClusterEye Metrics Implementation Guide

Bu dokümantasyon, ClusterEye'da InfluxDB ile time-series metrik toplama ve görüntüleme sisteminin kurulumu ve kullanımını açıklar.

## 📊 Özellikler

- **Real-time Metrik Toplama**: Agent'lardan gelen sistem metriklerini otomatik olarak InfluxDB'ye yazar
- **Time-series Veritabanı**: InfluxDB v2 ile optimize edilmiş metrik saklama
- **RESTful API**: Metrikleri sorgulamak için kapsamlı API endpoint'leri
- **Dashboard Desteği**: Frontend için optimize edilmiş metrik endpoint'leri
- **Flexible Querying**: Flux query language desteği

## 🏗️ Mimari

```
Agent → gRPC → ClusterEye Server → InfluxDB Writer → InfluxDB
                    ↓
               Metric API ← Frontend Dashboard
```

## 🚀 Kurulum

### 1. InfluxDB Kurulumu

#### Docker ile:
```bash
docker run -d \
  --name influxdb \
  -p 8086:8086 \
  -v influxdb-storage:/var/lib/influxdb2 \
  influxdb:2.7
```

#### Manuel Kurulum:
```bash
# Ubuntu/Debian
wget -qO- https://repos.influxdata.com/influxdb.key | sudo apt-key add -
echo "deb https://repos.influxdata.com/ubuntu focal stable" | sudo tee /etc/apt/sources.list.d/influxdb.list
sudo apt-get update && sudo apt-get install influxdb2
```

### 2. InfluxDB Konfigürasyonu

1. InfluxDB UI'ya gidin: `http://localhost:8086`
2. İlk kurulum sihirbazını tamamlayın:
   - Organization: `clustereye`
   - Bucket: `clustereye`
   - Username/Password belirleyin
3. API Token oluşturun:
   - Data → API Tokens → Generate API Token
   - Read/Write permissions verin

### 3. ClusterEye Konfigürasyonu

`server.yml` dosyasını düzenleyin:

```yaml
influxdb:
  url: http://localhost:8086
  token: "your-generated-token-here"
  organization: "clustereye"
  bucket: "clustereye"
  enabled: true
  batch_size: 1000
  flush_interval: 10
```

### 4. Servisi Başlatın

```bash
go run cmd/api/main.go
```

## 📈 Metrik Türleri

### Sistem Metrikleri

#### CPU Metrikleri
- **Measurement**: `cpu_usage`, `cpu_load`, `cpu_info`
- **Fields**: `usage_percent`, `cores`, `load_average`
- **Tags**: `agent_id`, `period`

#### Memory Metrikleri
- **Measurement**: `memory_usage`, `memory_info`
- **Fields**: `usage_percent`, `total_bytes`, `used_bytes`, `available_bytes`
- **Tags**: `agent_id`

#### Disk Metrikleri
- **Measurement**: `disk_usage`, `disk_info`
- **Fields**: `usage_percent`, `total_bytes`, `used_bytes`
- **Tags**: `agent_id`, `mountpoint`, `device`

#### Network Metrikleri
- **Measurement**: `network_io`, `network_packets`
- **Fields**: `bytes`, `packets`
- **Tags**: `agent_id`, `interface`, `direction`

### Database Metrikleri

#### PostgreSQL
- **Measurement**: `postgresql_connections`, `postgresql_database`, `postgresql_performance`
- **Fields**: `count`, `size_bytes`, `transactions_per_second`
- **Tags**: `agent_id`

#### MongoDB
- **Measurement**: `mongodb_connections`, `mongodb_performance`, `mongodb_memory`
- **Fields**: `count`, `operations_per_second`, `usage_bytes`
- **Tags**: `agent_id`

#### MSSQL
- **Measurement**: `mssql_connections`, `mssql_performance`
- **Fields**: `count`, `cpu_usage_percent`, `buffer_cache_hit_ratio`
- **Tags**: `agent_id`

## 🔌 API Endpoint'leri

### Genel Metrik Sorgulama
```http
POST /api/v1/metrics/query
Content-Type: application/json

{
  "query": "from(bucket: \"clustereye\") |> range(start: -1h) |> filter(fn: (r) => r._measurement == \"cpu_usage\")"
}
```

### Özel Metrik Endpoint'leri

#### CPU Metrikleri
```http
GET /api/v1/metrics/cpu?agent_id=agent_test-sql-01&range=1h
```

#### Memory Metrikleri
```http
GET /api/v1/metrics/memory?agent_id=agent_123&range=1h
```

#### Disk Metrikleri
```http
GET /api/v1/metrics/disk?agent_id=agent_123&range=1h
```

#### Network Metrikleri
```http
GET /api/v1/metrics/network?agent_id=agent_123&range=1h
```

#### Database Metrikleri
```http
GET /api/v1/metrics/database?agent_id=agent_123&type=postgresql&range=1h
```

#### Dashboard Metrikleri
```http
GET /api/v1/metrics/dashboard?range=1h
```

## 📊 Örnek Flux Sorguları

### Son 1 saatteki ortalama CPU kullanımı
```flux
from(bucket: "clustereye")
  |> range(start: -1h)
  |> filter(fn: (r) => r._measurement == "cpu_usage")
  |> filter(fn: (r) => r._field == "usage_percent")
  |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)
```

### Agent bazında memory kullanımı
```flux
from(bucket: "clustereye")
  |> range(start: -1h)
  |> filter(fn: (r) => r._measurement == "memory_usage")
  |> filter(fn: (r) => r._field == "usage_percent")
  |> group(columns: ["agent_id"])
  |> aggregateWindow(every: 10m, fn: mean, createEmpty: false)
```

### Disk kullanımı alarm seviyesi
```flux
from(bucket: "clustereye")
  |> range(start: -1h)
  |> filter(fn: (r) => r._measurement == "disk_usage")
  |> filter(fn: (r) => r._field == "usage_percent")
  |> filter(fn: (r) => r._value > 80.0)
```

## 🔧 Troubleshooting

### InfluxDB Bağlantı Sorunları

1. **Token Kontrolü**:
   ```bash
   curl -H "Authorization: Token YOUR_TOKEN" http://localhost:8086/api/v2/buckets
   ```

2. **Bucket Varlığı**:
   ```bash
   influx bucket list --token YOUR_TOKEN
   ```

3. **Log Kontrolü**:
   ```bash
   # ClusterEye logları
   tail -f /var/log/clustereye/server.log
   
   # InfluxDB logları
   docker logs influxdb
   ```

### Performans Optimizasyonu

1. **Batch Size Ayarı**:
   - Yüksek throughput için `batch_size: 5000`
   - Düşük latency için `batch_size: 100`

2. **Flush Interval**:
   - Real-time için `flush_interval: 1`
   - Batch processing için `flush_interval: 60`

3. **Retention Policy**:
   ```flux
   // 30 gün sonra eski verileri sil
   from(bucket: "clustereye")
     |> range(start: -30d)
     |> drop()
   ```

## 📚 Frontend Entegrasyonu

### React/Vue.js Örneği

```javascript
// CPU metriklerini al
const fetchCPUMetrics = async (agentId, timeRange = '1h') => {
  const response = await fetch(`/api/v1/metrics/cpu?agent_id=${agentId}&range=${timeRange}`);
  const data = await response.json();
  return data.data;
};

// Dashboard metrikleri
const fetchDashboardMetrics = async () => {
  const response = await fetch('/api/v1/metrics/dashboard?range=1h');
  const data = await response.json();
  return data.data;
};
```

### Chart.js Entegrasyonu

```javascript
// Time-series chart için veri formatı
const formatMetricsForChart = (metrics) => {
  return {
    labels: metrics.map(m => new Date(m._time)),
    datasets: [{
      label: 'CPU Usage %',
      data: metrics.map(m => m._value),
      borderColor: 'rgb(75, 192, 192)',
      tension: 0.1
    }]
  };
};
```

## 🔒 Güvenlik

1. **Token Güvenliği**:
   - Token'ları environment variable olarak saklayın
   - Read-only token'lar kullanın
   - Token rotation yapın

2. **Network Güvenliği**:
   - InfluxDB'yi internal network'te tutun
   - TLS/SSL kullanın
   - Firewall kuralları ayarlayın

## 📈 Monitoring

### InfluxDB Health Check
```bash
curl http://localhost:8086/health
```

### Metrik Yazma Durumu
```bash
curl -H "Authorization: Token YOUR_TOKEN" \
     "http://localhost:8086/api/v2/query" \
     -d 'from(bucket:"clustereye") |> range(start:-5m) |> count()'
```

## 🚀 Production Deployment

### Docker Compose Örneği

```yaml
version: '3.8'
services:
  influxdb:
    image: influxdb:2.7
    ports:
      - "8086:8086"
    volumes:
      - influxdb-storage:/var/lib/influxdb2
    environment:
      - DOCKER_INFLUXDB_INIT_MODE=setup
      - DOCKER_INFLUXDB_INIT_USERNAME=admin
      - DOCKER_INFLUXDB_INIT_PASSWORD=password123
      - DOCKER_INFLUXDB_INIT_ORG=clustereye
      - DOCKER_INFLUXDB_INIT_BUCKET=clustereye

  clustereye:
    build: .
    ports:
      - "8080:8080"
      - "50051:50051"
    depends_on:
      - influxdb
    environment:
      - INFLUXDB_URL=http://influxdb:8086
      - INFLUXDB_TOKEN=your-token
      - INFLUXDB_ORG=clustereye
      - INFLUXDB_BUCKET=clustereye

volumes:
  influxdb-storage:
```

Bu implementasyon ile ClusterEye artık güçlü bir time-series metrik toplama ve görüntüleme sistemine sahip! 