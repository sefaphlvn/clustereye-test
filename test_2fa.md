# 2FA (Two-Factor Authentication) Test Senaryoları

## Gereksinimler
- Google Authenticator veya benzeri TOTP uygulaması
- API test aracı (Postman, curl, vb.)

## 1. 2FA Durumunu Kontrol Etme

```bash
curl -X GET "http://localhost:8080/api/v1/2fa/status" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json"
```

**Beklenen Yanıt:**
```json
{
  "success": true,
  "enabled": false
}
```

## 2. 2FA Secret Oluşturma

```bash
curl -X POST "http://localhost:8080/api/v1/2fa/generate" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json"
```

**Beklenen Yanıt:**
```json
{
  "success": true,
  "secret": "JBSWY3DPEHPK3PXP",
  "qr_code": "otpauth://totp/ClusterEye:username%20(email)?secret=JBSWY3DPEHPK3PXP&issuer=ClusterEye",
  "manual_key": "JBSWY3DPEHPK3PXP"
}
```

## 3. QR Kodu Google Authenticator'a Ekleme

1. Google Authenticator uygulamasını açın
2. "+" butonuna tıklayın
3. "QR kod tarayın" seçeneğini seçin
4. Yukarıdaki `qr_code` URL'ini QR kod oluşturucu ile QR koda çevirin ve tarayın
5. Alternatif olarak "Manuel giriş" ile `manual_key` değerini girin

## 4. 2FA'yı Aktif Etme

Google Authenticator'dan aldığınız 6 haneli kodu kullanın:

```bash
curl -X POST "http://localhost:8080/api/v1/2fa/enable" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "code": "123456"
  }'
```

**Beklenen Yanıt:**
```json
{
  "success": true,
  "message": "2FA enabled successfully",
  "backup_codes": [
    "ABCD1234",
    "EFGH5678",
    "IJKL9012",
    "MNOP3456",
    "QRST7890",
    "UVWX1234",
    "YZAB5678",
    "CDEF9012",
    "GHIJ3456",
    "KLMN7890"
  ]
}
```

**Önemli:** Backup kodlarını güvenli bir yerde saklayın!

## 5. 2FA ile Login Testi

Artık login yaparken 2FA kodu gerekli:

```bash
# İlk adım: Kullanıcı adı ve şifre ile login
curl -X POST "http://localhost:8080/api/v1/login" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "admin",
    "password": "admin123"
  }'
```

**Beklenen Yanıt (2FA gerekli):**
```json
{
  "error": "2FA code required",
  "requires_2fa": true
}
```

```bash
# İkinci adım: 2FA kodu ile login
curl -X POST "http://localhost:8080/api/v1/login" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "admin",
    "password": "admin123",
    "twofa_code": "123456"
  }'
```

**Beklenen Yanıt (başarılı):**
```json
{
  "success": true,
  "user": {
    "username": "admin",
    "email": "admin@example.com",
    "admin": "true",
    "totp_enabled": true
  },
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

## 6. Backup Kod ile Login

Google Authenticator erişiminiz yoksa backup kod kullanabilirsiniz:

```bash
curl -X POST "http://localhost:8080/api/v1/login" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "admin",
    "password": "admin123",
    "twofa_code": "ABCD1234"
  }'
```

**Not:** Her backup kod sadece bir kez kullanılabilir.

## 7. Backup Kodları Yenileme

```bash
curl -X POST "http://localhost:8080/api/v1/2fa/regenerate-backup-codes" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "password": "admin123"
  }'
```

## 8. 2FA'yı Deaktif Etme

```bash
curl -X POST "http://localhost:8080/api/v1/2fa/disable" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "password": "admin123"
  }'
```

## Güvenlik Notları

1. **Secret Güvenliği:** TOTP secret'ları veritabanında güvenli şekilde saklanır
2. **Backup Kodları:** Her backup kod sadece bir kez kullanılabilir
3. **Şifre Doğrulama:** 2FA deaktif etme ve backup kod yenileme için şifre gereklidir
4. **Rate Limiting:** Üretim ortamında 2FA denemelerinde rate limiting uygulanmalıdır
5. **Audit Logging:** 2FA işlemleri loglanmalıdır

## Veritabanı Değişiklikleri

Migration dosyası (`008_add_2fa_to_users.sql`) aşağıdaki alanları users tablosuna ekler:

- `totp_secret`: TOTP secret anahtarı (VARCHAR(32))
- `totp_enabled`: 2FA aktif mi? (BOOLEAN, default: false)
- `backup_codes`: Backup kodları dizisi (TEXT[])

## QR Kod Oluşturma

Frontend'de QR kod göstermek için `qr_code` URL'ini kullanabilirsiniz:

```javascript
// JavaScript örneği
const qrCodeUrl = response.qr_code;
// QR kod kütüphanesi ile görselleştirin
```

## Hata Durumları

- **Invalid 2FA code:** Yanlış TOTP kodu
- **2FA secret not generated:** Secret oluşturulmadan enable edilmeye çalışılması
- **2FA is not enabled:** 2FA aktif değilken backup kod yenileme
- **Invalid password:** Yanlış şifre ile deaktif etme/backup kod yenileme 