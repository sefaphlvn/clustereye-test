# AWS RDS İzleme Özelliği - Kurulum ve Kullanım Kılavuzu

## 🚀 Genel Bakış

ClusterEye artık AWS RDS veritabanlarını izleme yeteneğine sahip! Bu özellik sayesinde:

- **CloudWatch Metrikleri**: CPU, Memory, IOPS, Latency, Network, Storage
- **Performance Insights**: Database Load, Wait Events, Top SQL Queries
- **RDS Instance Status**: Engine bilgileri, endpoint'ler, konfigürasyon

## 📋 Gereksinimler

### AWS Permissions

AWS RDS izleme için aşağıdaki IAM izinleri gereklidir:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "rds:DescribeDBInstances",
                "cloudwatch:GetMetricStatistics",
                "pi:GetResourceMetrics",
                "pi:DescribeDimensionKeys"
            ],
            "Resource": "*"
        }
    ]
}
```

### AWS Credentials

Aşağıdaki yöntemlerden birini kullanabilirsiniz:

1. **IAM Role** (Önerilen - EC2'de çalışıyorsa)
2. **Environment Variables**:
   ```bash
   export AWS_ACCESS_KEY_ID="your-access-key"
   export AWS_SECRET_ACCESS_KEY="your-secret-key"
   export AWS_REGION="us-east-1"
   ```
3. **Config dosyasında manuel tanımlama**

## ⚙️ Konfigürasyon

### 1. server.yml Dosyasını Güncelleyin

```yaml
# AWS RDS İzleme Ayarları
aws_rds:
  enabled: true  # AWS RDS izlemeyi aktif et
  region: "us-east-1"  # AWS region
  
  # AWS credentials (opsiyonel - IAM role kullanılabilir)
  access_key_id: ""  # Boş bırakın IAM role kullanmak için
  secret_access_key: ""  # Boş bırakın IAM role kullanmak için
  
  poll_interval_sec: 300  # 5 dakika (minimum 60 saniye)
  
  # Performance Insights ayarları
  performance_insights:
    enabled: true
  
  # İzlenecek RDS instance'ları
  monitor_instances:
    - instance_id: "my-postgres-db"  # RDS Instance Identifier
      resource_id: "db-ABCDEFGHIJKLMNOP"  # DBI Resource ID (Performance Insights için)
      engine: "postgresql"  # postgresql, mysql, sqlserver, oracle
      display_name: "Production PostgreSQL"
      enabled: true  # Bu instance'ı izle
    
    - instance_id: "my-mysql-db"
      resource_id: "db-QRSTUVWXYZ123456"
      engine: "mysql"
      display_name: "Production MySQL"
      enabled: true
```

### 2. RDS Instance Bilgilerini Bulma

#### Instance ID:
AWS Console > RDS > Databases > Instance adı

#### Resource ID (Performance Insights için):
```bash
aws rds describe-db-instances --db-instance-identifier your-instance-id \
  --query 'DBInstances[0].DbiResourceId' --output text
```

Veya AWS Console > RDS > Performance Insights > Configuration

## 🔧 Kurulum Adımları

### 1. Dependencies'i Yükleyin
```bash
go mod tidy
```

### 2. Konfigürasyonu Güncelleyin
`server.yml` dosyasını yukarıdaki örnekteki gibi güncelleyin.

### 3. InfluxDB'yi Başlatın
AWS RDS metrikleri InfluxDB'de saklanır:

```bash
# Docker ile InfluxDB başlatma
docker run -d -p 8086:8086 \
  -v influxdb-storage:/var/lib/influxdb2 \
  influxdb:2.7
```

### 4. ClusterEye Server'ı Başlatın
```bash
go run cmd/api/main.go
```

## 📊 API Endpoint'leri

### AWS RDS Metrikleri

| Endpoint | Açıklama | Parametreler |
|----------|----------|--------------|
| `GET /api/v1/metrics/aws-rds/instances` | RDS instance listesi | `range` (1h, 24h, 7d) |
| `GET /api/v1/metrics/aws-rds/cpu` | CPU metrikleri | `instance_id`, `range` |
| `GET /api/v1/metrics/aws-rds/connections` | Bağlantı sayısı | `instance_id`, `range` |
| `GET /api/v1/metrics/aws-rds/memory` | Memory kullanımı | `instance_id`, `range` |
| `GET /api/v1/metrics/aws-rds/iops` | IOPS metrikleri | `instance_id`, `range` |
| `GET /api/v1/metrics/aws-rds/latency` | Latency metrikleri | `instance_id`, `range` |
| `GET /api/v1/metrics/aws-rds/network` | Network throughput | `instance_id`, `range` |
| `GET /api/v1/metrics/aws-rds/storage` | Storage kullanımı | `instance_id`, `range` |
| `GET /api/v1/metrics/aws-rds/performance-insights` | Performance Insights | `instance_id`, `range` |
| `GET /api/v1/metrics/aws-rds/top-sql` | Top SQL queries | `instance_id`, `range` |
| `GET /api/v1/metrics/aws-rds/status` | Instance status | `instance_id`, `range` |

### Örnek API Çağrıları

```bash
# Tüm RDS instance'larını listele
curl "http://localhost:8080/api/v1/metrics/aws-rds/instances"

# Belirli bir instance'ın CPU metriklerini al
curl "http://localhost:8080/api/v1/metrics/aws-rds/cpu?instance_id=my-postgres-db&range=1h"

# Performance Insights metriklerini al
curl "http://localhost:8080/api/v1/metrics/aws-rds/performance-insights?instance_id=my-postgres-db&range=24h"

# Top SQL queries'i al
curl "http://localhost:8080/api/v1/metrics/aws-rds/top-sql?instance_id=my-postgres-db&range=1h"
```

## 📈 İzlenen Metrikler

### CloudWatch Metrikleri

#### Temel Metrikler (Tüm Engine'ler):
- **CPU**: CPUUtilization
- **Memory**: FreeableMemory
- **Connections**: DatabaseConnections
- **IOPS**: ReadIOPS, WriteIOPS
- **Latency**: ReadLatency, WriteLatency
- **Network**: NetworkReceiveThroughput, NetworkTransmitThroughput
- **Storage**: FreeStorageSpace

#### Engine-Specific Metrikler:

**PostgreSQL**:
- MaximumUsedTransactionIDs
- TransactionLogsDiskUsage

**MySQL**:
- BinLogDiskUsage

**SQL Server**:
- FailedSQLServerAgentJobsCount

### Performance Insights Metrikleri

#### Database Load:
- `db.load.avg` - Ortalama database load
- `db.load.peak` - Peak database load

#### PostgreSQL Wait Events:
- `db.wait.lock.avg` - Lock wait time
- `db.wait.lwlock.avg` - Lightweight lock wait time
- `db.wait.bufferpin.avg` - Buffer pin wait time

#### MySQL Wait Events:
- `db.wait.io.avg` - I/O wait time

#### SQL Server Wait Events:
- `db.wait.pageio.avg` - Page I/O wait time
- `db.wait.locks.avg` - Lock wait time

#### Transaction Metrikleri:
- `db.Transactions.xact_commit.avg` (PostgreSQL)
- `db.Transactions.xact_rollback.avg` (PostgreSQL)
- `db.SQL.Select.avg` (MySQL)
- `db.SQL.Insert.avg` (MySQL)
- `db.SQL.batch_requests.avg` (SQL Server)

## 🔍 Troubleshooting

### 1. AWS Credentials Hatası
```
AWS config yüklenemedi: NoCredentialProviders
```

**Çözüm**: AWS credentials'ları kontrol edin:
```bash
aws configure list
aws sts get-caller-identity
```

### 2. Performance Insights Hatası
```
Performance Insights metrikleri alınamadı: AccessDenied
```

**Çözüm**: 
- Performance Insights'ın RDS instance'da aktif olduğunu kontrol edin
- IAM izinlerinde `pi:*` permission'larını kontrol edin

### 3. Resource ID Bulunamadı
```
DBI Resource ID bulunamadı
```

**Çözüm**: 
```bash
aws rds describe-db-instances --db-instance-identifier YOUR_INSTANCE_ID \
  --query 'DBInstances[0].DbiResourceId'
```

### 4. Metrik Verisi Yok
```
metrik verisi bulunamadı
```

**Çözüm**:
- RDS instance'ın çalışır durumda olduğunu kontrol edin
- CloudWatch'ta metriklerin mevcut olduğunu kontrol edin
- Time range'i artırın (örn: 1h yerine 24h)

## 🎯 Best Practices

### 1. Poll Interval
- **Minimum**: 60 saniye (AWS API rate limits)
- **Önerilen**: 300 saniye (5 dakika)
- **Production**: 600 saniye (10 dakika) - maliyet optimizasyonu için

### 2. Instance Selection
- Sadece kritik production instance'ları izleyin
- Development/test instance'ları için `enabled: false` yapın

### 3. Performance Insights
- Sadece gerekli instance'lar için aktif edin (ek maliyet)
- Resource ID'yi doğru şekilde konfigüre edin

### 4. IAM Security
- Minimum gerekli izinleri verin
- IAM role kullanmayı tercih edin (access key yerine)

## 💰 Maliyet Optimizasyonu

### CloudWatch API Calls
- Her metrik için ayrı API call yapılır
- 10 metrik × 12 instance × 24 saat = 2,880 API call/gün
- Poll interval'i artırarak maliyeti düşürebilirsiniz

### Performance Insights
- Ek ücretlidir ($0.02/vCPU/saat)
- Sadece gerekli instance'lar için aktif edin

## 🔄 Monitoring ve Alerting

### InfluxDB Queries

```sql
-- CPU kullanımı %80 üzeri
from(bucket: "clustereye")
  |> range(start: -1h)
  |> filter(fn: (r) => r._measurement == "aws_rds_cpu")
  |> filter(fn: (r) => r._field == "CPUUtilization")
  |> filter(fn: (r) => r._value > 80)

-- Bağlantı sayısı trend
from(bucket: "clustereye")
  |> range(start: -24h)
  |> filter(fn: (r) => r._measurement == "aws_rds_connections")
  |> filter(fn: (r) => r._field == "DatabaseConnections")
  |> aggregateWindow(every: 1h, fn: mean)
```

### Grafana Dashboard

AWS RDS metrikleri için Grafana dashboard oluşturabilirsiniz:

1. **CPU Panel**: `aws_rds_cpu` measurement
2. **Memory Panel**: `aws_rds_memory` measurement  
3. **IOPS Panel**: `aws_rds_iops` measurement
4. **Top SQL Panel**: `aws_rds_top_sql` measurement

## 📝 Changelog

### v1.0.0
- ✅ CloudWatch temel metrikleri
- ✅ Performance Insights entegrasyonu
- ✅ Multi-engine support (PostgreSQL, MySQL, SQL Server)
- ✅ RESTful API endpoints
- ✅ InfluxDB storage
- ✅ Configurable polling intervals

## 🤝 Katkıda Bulunma

1. Feature request'ler için GitHub issue açın
2. Bug report'lar için detaylı log'ları paylaşın
3. Pull request'ler için test coverage'ı sağlayın

## 📞 Destek

- **Documentation**: Bu dosya
- **Issues**: GitHub Issues
- **Email**: support@clustereye.com 