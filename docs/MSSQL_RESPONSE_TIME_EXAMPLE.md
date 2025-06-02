# SQL Server Response Time Ölçümü - Agent Implementation Örneği

Bu dokümanda, SQL Server'a basit bir `SELECT 1` sorgusu göndererek response time ölçümü nasıl yapılacağını açıklıyoruz.

## Metrik Formatı

Agent'ın server'a göndermesi gereken response time metriği formatı:

```json
{
  "cpu_usage": 45.2,
  "memory_usage": 62.8,
  "total_memory": 8589934592,
  "free_memory": 3221225472,
  "total_disk": 107374182400,
  "free_disk": 53687091200,
  "cpu_cores": 4,
  "response_time_ms": 1.25
}
```

## Python Implementation Örneği

```python
import time
import pyodbc
from typing import Optional

class SQLServerResponseTimeCollector:
    def __init__(self, connection_string: str):
        """
        SQL Server response time ölçümü için collector sınıfı
        
        Args:
            connection_string: SQL Server bağlantı string'i
                Örnek: "DRIVER={ODBC Driver 17 for SQL Server};SERVER=localhost;DATABASE=master;UID=sa;PWD=password"
        """
        self.connection_string = connection_string
        
    def measure_response_time(self) -> Optional[float]:
        """
        SELECT 1 sorgusu ile response time ölçümü yapar
        
        Returns:
            float: Response time millisecond cinsinden, hata durumunda None
        """
        try:
            # Başlangıç zamanını kaydet
            start_time = time.time()
            
            # SQL Server'a bağlan
            with pyodbc.connect(self.connection_string, timeout=5) as conn:
                with conn.cursor() as cursor:
                    # Basit SELECT 1 sorgusu çalıştır
                    cursor.execute("SELECT 1")
                    result = cursor.fetchone()
                    
                    # Sonucu kontrol et (opsiyonel)
                    if result and result[0] == 1:
                        # Bitiş zamanını hesapla
                        end_time = time.time()
                        
                        # Millisecond cinsinden döndür
                        duration_ms = (end_time - start_time) * 1000
                        return round(duration_ms, 2)
                        
        except Exception as e:
            print(f"Response time ölçümü hatası: {e}")
            return None
            
    def collect_metrics_with_response_time(self) -> dict:
        """
        Diğer sistem metrikleri ile birlikte response time'ı toplar
        
        Returns:
            dict: Tüm metrikleri içeren sözlük
        """
        # Response time ölç
        response_time = self.measure_response_time()
        
        # Diğer sistem metriklerini topla (örnek değerler)
        metrics = {
            "cpu_usage": self.get_cpu_usage(),
            "memory_usage": self.get_memory_usage(),
            "total_memory": self.get_total_memory(),
            "free_memory": self.get_free_memory(),
            "total_disk": self.get_total_disk(),
            "free_disk": self.get_free_disk(),
            "cpu_cores": self.get_cpu_cores(),
        }
        
        # Response time'ı ekle (sadece başarılı ölçümler için)
        if response_time is not None:
            metrics["response_time_ms"] = response_time
            
        return metrics
    
    def get_cpu_usage(self) -> float:
        # CPU kullanım yüzdesini döndür
        # Implement your CPU monitoring logic here
        return 45.2
    
    def get_memory_usage(self) -> float:
        # Memory kullanım yüzdesini döndür
        # Implement your memory monitoring logic here
        return 62.8
    
    def get_total_memory(self) -> int:
        # Toplam memory'yi byte cinsinden döndür
        # Implement your memory size detection logic here
        return 8589934592  # 8GB
    
    def get_free_memory(self) -> int:
        # Boş memory'yi byte cinsinden döndür
        # Implement your free memory detection logic here
        return 3221225472  # 3GB
    
    def get_total_disk(self) -> int:
        # Toplam disk alanını byte cinsinden döndür
        # Implement your disk size detection logic here
        return 107374182400  # 100GB
    
    def get_free_disk(self) -> int:
        # Boş disk alanını byte cinsinden döndür
        # Implement your free disk space detection logic here
        return 53687091200  # 50GB
    
    def get_cpu_cores(self) -> int:
        # CPU core sayısını döndür
        # Implement your CPU core detection logic here
        return 4

# Kullanım örneği
if __name__ == "__main__":
    # Bağlantı string'ini yapılandır
    conn_str = "DRIVER={ODBC Driver 17 for SQL Server};SERVER=localhost;DATABASE=master;UID=sa;PWD=your_password"
    
    # Collector'ı başlat
    collector = SQLServerResponseTimeCollector(conn_str)
    
    # Tek seferlik response time ölçümü
    response_time = collector.measure_response_time()
    if response_time:
        print(f"SQL Server Response Time: {response_time} ms")
    else:
        print("Response time ölçümü başarısız")
    
    # Tüm metrikleri topla
    all_metrics = collector.collect_metrics_with_response_time()
    print("Toplanan metrikler:", all_metrics)
```

## Go Implementation Örneği

```go
package main

import (
    "database/sql"
    "fmt"
    "time"
    
    _ "github.com/denisenkom/go-mssqldb"
)

type SQLServerMetrics struct {
    CPUUsage      float64 `json:"cpu_usage"`
    MemoryUsage   float64 `json:"memory_usage"`
    TotalMemory   int64   `json:"total_memory"`
    FreeMemory    int64   `json:"free_memory"`
    TotalDisk     int64   `json:"total_disk"`
    FreeDisk      int64   `json:"free_disk"`
    CPUCores      int     `json:"cpu_cores"`
    ResponseTimeMs *float64 `json:"response_time_ms,omitempty"`
}

type ResponseTimeCollector struct {
    connectionString string
}

func NewResponseTimeCollector(connectionString string) *ResponseTimeCollector {
    return &ResponseTimeCollector{
        connectionString: connectionString,
    }
}

func (r *ResponseTimeCollector) MeasureResponseTime() (*float64, error) {
    // Başlangıç zamanını kaydet
    start := time.Now()
    
    // SQL Server'a bağlan
    db, err := sql.Open("sqlserver", r.connectionString)
    if err != nil {
        return nil, fmt.Errorf("veritabanı bağlantı hatası: %v", err)
    }
    defer db.Close()
    
    // Bağlantıyı test et
    if err := db.Ping(); err != nil {
        return nil, fmt.Errorf("veritabanı ping hatası: %v", err)
    }
    
    // SELECT 1 sorgusu çalıştır
    var result int
    err = db.QueryRow("SELECT 1").Scan(&result)
    if err != nil {
        return nil, fmt.Errorf("sorgu hatası: %v", err)
    }
    
    // Süreyi hesapla
    duration := time.Since(start)
    durationMs := float64(duration.Nanoseconds()) / 1e6 // Millisecond'a çevir
    
    // Sonucu kontrol et
    if result != 1 {
        return nil, fmt.Errorf("beklenmeyen sorgu sonucu: %d", result)
    }
    
    return &durationMs, nil
}

func (r *ResponseTimeCollector) CollectAllMetrics() SQLServerMetrics {
    metrics := SQLServerMetrics{
        CPUUsage:    r.getCPUUsage(),
        MemoryUsage: r.getMemoryUsage(),
        TotalMemory: r.getTotalMemory(),
        FreeMemory:  r.getFreeMemory(),
        TotalDisk:   r.getTotalDisk(),
        FreeDisk:    r.getFreeDisk(),
        CPUCores:    r.getCPUCores(),
    }
    
    // Response time ölç ve ekle
    if responseTime, err := r.MeasureResponseTime(); err == nil {
        metrics.ResponseTimeMs = responseTime
    } else {
        fmt.Printf("Response time ölçümü başarısız: %v\n", err)
    }
    
    return metrics
}

// Örnek implementasyonlar (gerçek değerler için system API'leri kullanın)
func (r *ResponseTimeCollector) getCPUUsage() float64 {
    // CPU kullanım yüzdesini döndür
    return 45.2
}

func (r *ResponseTimeCollector) getMemoryUsage() float64 {
    // Memory kullanım yüzdesini döndür
    return 62.8
}

func (r *ResponseTimeCollector) getTotalMemory() int64 {
    // Toplam memory'yi byte cinsinden döndür
    return 8589934592 // 8GB
}

func (r *ResponseTimeCollector) getFreeMemory() int64 {
    // Boş memory'yi byte cinsinden döndür
    return 3221225472 // 3GB
}

func (r *ResponseTimeCollector) getTotalDisk() int64 {
    // Toplam disk alanını byte cinsinden döndür
    return 107374182400 // 100GB
}

func (r *ResponseTimeCollector) getFreeDisk() int64 {
    // Boş disk alanını byte cinsinden döndür
    return 53687091200 // 50GB
}

func (r *ResponseTimeCollector) getCPUCores() int {
    // CPU core sayısını döndür
    return 4
}

func main() {
    // Bağlantı string'ini yapılandır
    connStr := "sqlserver://sa:your_password@localhost:1433?database=master"
    
    // Collector'ı başlat
    collector := NewResponseTimeCollector(connStr)
    
    // Tek seferlik response time ölçümü
    if responseTime, err := collector.MeasureResponseTime(); err == nil {
        fmt.Printf("SQL Server Response Time: %.2f ms\n", *responseTime)
    } else {
        fmt.Printf("Response time ölçümü başarısız: %v\n", err)
    }
    
    // Tüm metrikleri topla
    allMetrics := collector.CollectAllMetrics()
    fmt.Printf("Toplanan metrikler: %+v\n", allMetrics)
}
```

## C# Implementation Örneği

```csharp
using System;
using System.Data.SqlClient;
using System.Diagnostics;

public class SQLServerResponseTimeCollector
{
    private readonly string _connectionString;
    
    public SQLServerResponseTimeCollector(string connectionString)
    {
        _connectionString = connectionString;
    }
    
    public double? MeasureResponseTime()
    {
        try
        {
            var stopwatch = Stopwatch.StartNew();
            
            using (var connection = new SqlConnection(_connectionString))
            {
                connection.Open();
                
                using (var command = new SqlCommand("SELECT 1", connection))
                {
                    var result = command.ExecuteScalar();
                    stopwatch.Stop();
                    
                    if (result != null && (int)result == 1)
                    {
                        return stopwatch.Elapsed.TotalMilliseconds;
                    }
                }
            }
        }
        catch (Exception ex)
        {
            Console.WriteLine($"Response time ölçümü hatası: {ex.Message}");
        }
        
        return null;
    }
    
    public object CollectAllMetrics()
    {
        var responseTime = MeasureResponseTime();
        
        var metrics = new
        {
            cpu_usage = GetCPUUsage(),
            memory_usage = GetMemoryUsage(),
            total_memory = GetTotalMemory(),
            free_memory = GetFreeMemory(),
            total_disk = GetTotalDisk(),
            free_disk = GetFreeDisk(),
            cpu_cores = GetCPUCores(),
            response_time_ms = responseTime
        };
        
        return metrics;
    }
    
    private double GetCPUUsage() => 45.2; // Implement actual logic
    private double GetMemoryUsage() => 62.8; // Implement actual logic  
    private long GetTotalMemory() => 8589934592; // Implement actual logic
    private long GetFreeMemory() => 3221225472; // Implement actual logic
    private long GetTotalDisk() => 107374182400; // Implement actual logic
    private long GetFreeDisk() => 53687091200; // Implement actual logic
    private int GetCPUCores() => 4; // Implement actual logic
}
```

## Performans Önerileri

1. **Connection Pooling**: Database bağlantısı için connection pooling kullanın
2. **Timeout Ayarları**: Bağlantı timeout'ları uygun şekilde ayarlayın (önerilen: 5 saniye)
3. **Error Handling**: Network ve database hatalarını düzgün şekilde handle edin
4. **Measurement Frequency**: Response time ölçümünü çok sık yapmayın (önerilen: 30-60 saniye aralıklar)
5. **Connection Reuse**: Mümkünse aynı connection'ı yeniden kullanın

## API Endpoint'leri

Response time metriklerini sorgulamak için yeni endpoint'ler:

- **GET** `/api/v1/mssql/response-time?agent_id={agent_id}&range={timeRange}`
  - SQL Server response time metriklerini getirir
  - Örnek yanıt istatistikleri: latest, avg, min, max değerleri

- **GET** `/api/v1/metrics/dashboard?range={timeRange}`
  - Dashboard için tüm metrikleri getirir (response_time dahil)

## Örnek API Kullanımı

```bash
# Belirli bir agent'ın response time metriklerini al
curl "http://localhost:8080/api/v1/mssql/response-time?agent_id=agent-123&range=1h"

# Dashboard metriklerini al (response time dahil)
curl "http://localhost:8080/api/v1/metrics/dashboard?range=6h"
```

Bu implementasyon ile SQL Server response time ölçümü sisteme entegre edilmiş olur ve InfluxDB'de `mssql_response_time` ve `mssql_system` measurement'larında saklanır. 