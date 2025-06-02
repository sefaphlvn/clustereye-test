# AWS RDS İçin IAM Kurulumu

## 1. IAM Kullanıcı Oluşturma

AWS Console'a gidin ve IAM servisini açın:

1. **Users** → **Add users**
2. **User name**: `clustereye-rds-monitor` 
3. **Select AWS credential type**: ✅ **Access key - Programmatic access**
4. **Next: Permissions**

## 2. IAM Policy Oluşturma

**Create policy** tıklayın ve JSON sekmesinde aşağıdaki policy'yi ekleyin:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "RDSMonitoring",
            "Effect": "Allow",
            "Action": [
                "rds:DescribeDBInstances",
                "rds:DescribeDBClusters",
                "rds:DescribeDBParameterGroups",
                "rds:DescribeDBParameters",
                "rds:DescribeEvents"
            ],
            "Resource": "*"
        },
        {
            "Sid": "CloudWatchMetrics",
            "Effect": "Allow",
            "Action": [
                "cloudwatch:GetMetricStatistics",
                "cloudwatch:GetMetricData",
                "cloudwatch:ListMetrics"
            ],
            "Resource": "*"
        },
        {
            "Sid": "PerformanceInsights",
            "Effect": "Allow",
            "Action": [
                "pi:GetResourceMetrics",
                "pi:DescribeDimensionKeys",
                "pi:GetDimensionKeyDetails"
            ],
            "Resource": "*"
        }
    ]
}
```

**Policy name**: `ClusterEye-RDS-Monitoring-Policy`

## 3. Policy'yi Kullanıcıya Atama

1. Kullanıcı oluştururken **Attach existing policies directly**
2. `ClusterEye-RDS-Monitoring-Policy`'yi seçin
3. **Next: Tags** → **Next: Review** → **Create user**

## 4. Access Keys'i Kaydetme

✅ **Access key ID** ve **Secret access key**'i güvenli bir yerde saklayın!

---

## Alternatif: EC2 IAM Role (Eğer ClusterEye EC2'de çalışıyorsa)

Eğer ClusterEye uygulamanız EC2 instance'da çalışıyorsa, IAM Role kullanabilirsiniz:

1. **IAM** → **Roles** → **Create role**
2. **AWS service** → **EC2** → **Next**
3. Yukarıdaki policy'yi attach edin
4. **Role name**: `ClusterEye-RDS-Role`
5. EC2 instance'ınıza bu role'ü attach edin

Bu durumda AWS credentials (Access Key/Secret) gerekmez.

---

# RDS Instance Bilgilerini Bulma

## 1. RDS Console'dan Instance ID Bulma

1. **AWS Console** → **RDS** → **Databases**
2. MSSQL instance'ınızı seçin
3. **DB instance identifier** (Örnek: `my-mssql-db`)
4. **Resource ID** için **Configuration** sekmesine bakın (Örnek: `db-ABCD1234567890`)

## 2. AWS CLI ile Bilgileri Alma (Opsiyonel)

```bash
aws rds describe-db-instances --db-instance-identifier your-instance-name
```

Bu komut şu bilgileri verir:
- **DBInstanceIdentifier**: Configuration'da kullanacağınız ID
- **DbiResourceId**: Performance Insights için Resource ID

---

# Performance Insights Kurulumu

## 1. Performance Insights'ı Aktifleştirme

**RDS Console'da:**

1. MSSQL instance'ınızı seçin
2. **Modify** tıklayın
3. **Performance Insights** bölümünde:
   - ✅ **Enable Performance Insights**
   - **Retention period**: 7 days (free) veya daha uzun (ücretli)
4. **Continue** → **Apply immediately** → **Modify DB instance**

## 2. Değişikliklerin Uygulanmasını Bekleyin

- Modificasyon işlemi birkaç dakika sürebilir
- Instance durumu **modifying** → **available** olana kadar bekleyin

## 3. Performance Insights Kontrol

- RDS Console'da instance'ı seçin
- **Performance Insights** sekmesi görünmeli
- Eğer aktifse, grafikleri görebilirsiniz

---

# CloudWatch Enhanced Monitoring (Opsiyonel)

Daha detaylı sistem metrikleri için:

1. RDS instance'ı **Modify** edin
2. **Monitoring** bölümünde:
   - ✅ **Enable Enhanced monitoring**
   - **Granularity**: 60 seconds
3. **Apply** edin

Bu, işletim sistemi seviyesinde metrikler sağlar (CPU, Memory, File System, etc.).

---

# ClusterEye Konfigürasyonu

AWS kurulumunu tamamladıktan sonra, `server.yml` dosyanızı güncelleyin:

## Örnek Konfigürasyon

```yaml
aws_rds:
  enabled: true
  region: "us-east-1"  # RDS instance'ınızın region'ı
  
  # IAM User kullanıyorsanız:
  access_key_id: "AKIA..."
  secret_access_key: "your-secret-key"
  
  # IAM Role kullanıyorsanız bu satırları boş bırakın veya silin
  
  poll_interval_sec: 300  # 5 dakika
  
  performance_insights:
    enabled: true
  
  monitor_instances:
    - instance_id: "your-mssql-instance-id"        # RDS Console'dan alın
      resource_id: "db-ABCD1234567890"             # RDS Console'dan alın  
      engine: "sqlserver"
      display_name: "Production MSSQL"
      enabled: true
```

## Environment Variables Kullanımı (Daha Güvenli)

Alternatif olarak environment variables kullanabilirsiniz:

```bash
export AWS_REGION="us-east-1"
export AWS_ACCESS_KEY_ID="AKIA..."
export AWS_SECRET_ACCESS_KEY="your-secret-key"
export AWS_RDS_ENABLED="true"
export AWS_RDS_PI_ENABLED="true"
```

Bu durumda `server.yml`'de sadece instance bilgilerini belirtin:

```yaml
aws_rds:
  monitor_instances:
    - instance_id: "your-mssql-instance-id"
      resource_id: "db-ABCD1234567890"  
      engine: "sqlserver"
      display_name: "Production MSSQL"
      enabled: true
```

---

# Test Etme

ClusterEye'ı başlattıktan sonra logları kontrol edin:

```bash
# Application loglarında şunları görmelisiniz:
# [INFO] AWS RDS monitor başlatıldı - Region: us-east-1, Instances: 1
# [INFO] AWS RDS monitoring başlatıldı - Poll interval: 300 saniye
# [DEBUG] your-mssql-instance-id için X AWS RDS metriği yazıldı
```

API endpoint'lerini test edin:
- `GET /api/v1/metrics/aws-rds/instances` - Instance'ları listele
- `GET /api/v1/metrics/aws-rds/cpu` - CPU metrikleri
- `GET /api/v1/metrics/aws-rds/status` - Instance durumu 