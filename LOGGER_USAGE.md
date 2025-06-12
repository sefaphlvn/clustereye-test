# ClusterEye Logger Sistemi

## Genel Bakış

ClusterEye projesi artık yapılandırılabilir bir logger mekanizması kullanıyor. Bu sistem log seviyelerini, formatlarını ve çıktı hedeflerini esnek bir şekilde yönetmeyi sağlar.

## Konfigürasyon

Logger ayarları `server.yml` dosyasında yapılandırılır:

```yaml
log:
  level: info              # debug, info, warn, error
  format: json             # json, text
  output: both             # console, file, both
  file_path: logs/clustereye.log
  max_size: 100            # MB - dosya boyutu limiti
  max_backups: 5           # backup dosya sayısı
  max_age: 30              # gün - dosya saklama süresi
  compress: true           # eski dosyaları sıkıştır
```

### Konfigürasyon Seçenekleri

#### Log Seviyeleri
- **debug**: Tüm log mesajları (çok ayrıntılı)
- **info**: Bilgilendirme, uyarı ve hata mesajları
- **warn**: Uyarı ve hata mesajları
- **error**: Sadece hata mesajları

#### Format Seçenekleri
- **json**: Structured JSON format (production önerisi)
- **text**: İnsan okuyabilir format (development önerisi)

#### Output Seçenekleri
- **console**: Sadece konsol çıktısı
- **file**: Sadece dosya çıktısı
- **both**: Hem konsol hem dosya çıktısı

## Kullanım Örnekleri

### Temel Kullanım

```go
import "github.com/sefaphlvn/clustereye-test/internal/logger"

// Basit mesajlar
logger.Info().Msg("Sunucu başlatıldı")
logger.Error().Msg("Veritabanı bağlantısı hatası")
logger.Debug().Msg("Debug bilgisi")
logger.Warn().Msg("Uyarı mesajı")
```

### Structured Logging

```go
// Contextualized logging
logger.Info().
    Str("username", "admin").
    Str("ip", "192.168.1.100").
    Str("endpoint", "/login").
    Msg("Kullanıcı giriş yaptı")

logger.Error().
    Str("component", "database").
    Str("operation", "connect").
    Err(err).
    Msg("Veritabanı bağlantısı başarısız")

logger.Warn().
    Str("agent_id", "agent-123").
    Int("retry_count", 3).
    Msg("Agent bağlantısı yeniden deneniyor")
```

### Çeşitli Veri Tipleri

```go
logger.Info().
    Str("string_field", "değer").
    Int("int_field", 42).
    Bool("bool_field", true).
    Float64("float_field", 3.14).
    Time("timestamp", time.Now()).
    Dur("duration", time.Second*5).
    Msg("Çeşitli veri tipleri")
```

## JSON Output Örneği

```json
{
  "level": "info",
  "username": "admin", 
  "ip": "192.168.1.100",
  "endpoint": "/login",
  "time": "2025-06-12T18:45:22+03:00",
  "caller": "/home/user/project/internal/api/auth.go:156",
  "message": "Kullanıcı giriş yaptı"
}
```

## Text Output Örneği

```
2025-06-12T18:45:22+03:00 | INFO  | /internal/api/auth.go:156 > ***Kullanıcı giriş yaptı**** username:ADMIN ip:192.168.1.100 endpoint:/LOGIN
```

## Mevcut log.Printf Geçişi

Eski kullanım:
```go
log.Printf("User %s logged in from %s", username, ip)
```

Yeni kullanım:
```go
logger.Info().
    Str("username", username).
    Str("ip", ip).
    Msg("User logged in")
```

## Performans İpuçları

1. **Production'da JSON format kullanın** - Parse edilmesi daha kolay
2. **Debug level'ı sadece development'ta kullanın** - Performans etkisi var
3. **Structured fields kullanın** - Log analizi kolaylaşır
4. **File rotation ayarları** - Disk alanı kontrolü için önemli

## Log Dosyası Yönetimi

- Log dosyaları `logs/` dizininde saklanır
- Dosya boyutu `max_size` MB'a ulaştığında yeni dosya oluşturulur
- En fazla `max_backups` kadar eski dosya saklanır
- `max_age` günden eski dosyalar silinir
- `compress: true` ile eski dosyalar gzip ile sıkıştırılır

## Debug Önerileri

Development sırasında:
```yaml
log:
  level: debug
  format: text
  output: console
```

Production'da:
```yaml
log:
  level: info
  format: json
  output: both
  file_path: logs/clustereye.log
``` 