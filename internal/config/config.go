package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/sefaphlvn/clustereye-test/internal/database"
	"gopkg.in/yaml.v2"
)

// LogConfig, loglama ayarları
type LogConfig struct {
	Level      string `yaml:"level"`       // debug, info, warn, error
	Format     string `yaml:"format"`      // json, text
	Output     string `yaml:"output"`      // console, file, both
	FilePath   string `yaml:"file_path"`   // log dosyasının yolu
	MaxSize    int    `yaml:"max_size"`    // MB cinsinden maksimum dosya boyutu
	MaxBackups int    `yaml:"max_backups"` // tutulacak maksimum eski dosya sayısı
	MaxAge     int    `yaml:"max_age"`     // gün cinsinden maksimum yaş
	Compress   bool   `yaml:"compress"`    // eski dosyaları sıkıştır
}

// ServerConfig, ClusterEye sunucusu için konfigürasyon yapısı
type ServerConfig struct {
	// Server bilgileri
	Server struct {
		Name string `yaml:"name"`
	} `yaml:"server"`

	// GRPC Ayarları
	GRPC struct {
		Address string `yaml:"address"`
	} `yaml:"grpc"`

	// HTTP API Ayarları
	HTTP struct {
		Address string `yaml:"address"`
	} `yaml:"http"`

	// Log Ayarları
	Log LogConfig `yaml:"log"`

	// Veritabanı ayarları
	Database database.Config `yaml:"database"`

	// InfluxDB Configuration
	InfluxDB InfluxDBConfig `yaml:"influxdb"`
}

// AgentConfig, Agent için konfigürasyon yapısı
type AgentConfig struct {
	// Temel Bilgiler
	Key  string `yaml:"key"`
	Name string `yaml:"name"`

	// PostgreSQL Bağlantı Bilgileri
	PostgreSQL struct {
		Host    string `yaml:"host"`
		User    string `yaml:"user"`
		Pass    string `yaml:"pass"`
		Port    string `yaml:"port"`
		Cluster string `yaml:"cluster"`
		Auth    bool   `yaml:"-"` // Auth, dolaylı olarak belirlenir
	} `yaml:"postgresql"`

	// MongoDB Bağlantı Bilgileri
	Mongo struct {
		User    string `yaml:"user"`
		Pass    string `yaml:"pass"`
		Port    string `yaml:"port"`
		Replset string `yaml:"replset"`
		Auth    bool   `yaml:"-"` // Auth, dolaylı olarak belirlenir
	} `yaml:"mongo"`

	// GRPC Ayarları
	GRPC struct {
		ServerAddress string `yaml:"server_address"`
	} `yaml:"grpc"`
}

// InfluxDBConfig, InfluxDB bağlantı ayarları
type InfluxDBConfig struct {
	URL           string `yaml:"url" env:"INFLUXDB_URL"`
	Token         string `yaml:"token" env:"INFLUXDB_TOKEN"`
	Organization  string `yaml:"organization" env:"INFLUXDB_ORG"`
	Bucket        string `yaml:"bucket" env:"INFLUXDB_BUCKET"`
	Enabled       bool   `yaml:"enabled" env:"INFLUXDB_ENABLED"`
	BatchSize     int    `yaml:"batch_size" env:"INFLUXDB_BATCH_SIZE"`
	FlushInterval int    `yaml:"flush_interval" env:"INFLUXDB_FLUSH_INTERVAL"` // seconds
}

// LoadServerConfig, sunucu konfigürasyonunu yükler
func LoadServerConfig() (*ServerConfig, error) {
	// Konfigürasyon dosyasının yerini belirle
	configPath := getConfigPath("server.yml")

	// Dosya var mı kontrol et
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Dosya yoksa varsayılan konfigürasyonu oluştur ve kaydet
		return createDefaultServerConfig(configPath)
	}

	// Dosyayı oku
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("konfigürasyon dosyası okunamadı: %w", err)
	}

	// YAML'ı parse et
	var config ServerConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("konfigürasyon dosyası ayrıştırılamadı: %w", err)
	}

	return &config, nil
}

// LoadAgentConfig, agent konfigürasyonunu yükler
func LoadAgentConfig() (*AgentConfig, error) {
	// Konfigürasyon dosyasının yerini belirle
	configPath := getConfigPath("agent.yml")

	// Dosya var mı kontrol et
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Dosya yoksa varsayılan konfigürasyonu oluştur ve kaydet
		return createDefaultAgentConfig(configPath)
	}

	// Dosyayı oku
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("konfigürasyon dosyası okunamadı: %w", err)
	}

	// YAML'ı parse et
	var config AgentConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("konfigürasyon dosyası ayrıştırılamadı: %w", err)
	}

	// Auth değerlerini belirle
	config.PostgreSQL.Auth = config.PostgreSQL.User != "" && config.PostgreSQL.Pass != ""
	config.Mongo.Auth = config.Mongo.User != "" && config.Mongo.Pass != ""

	return &config, nil
}

// createDefaultServerConfig, varsayılan sunucu konfigürasyonunu oluşturur
func createDefaultServerConfig(configPath string) (*ServerConfig, error) {
	// Varsayılan ayarlar
	config := &ServerConfig{}
	config.Server.Name = "ClusterEye"
	config.GRPC.Address = ":50051"
	config.HTTP.Address = ":8080"

	// Log varsayılan ayarları
	config.Log = LogConfig{
		Level:      "info",
		Format:     "json",
		Output:     "both",
		FilePath:   "logs/clustereye.log",
		MaxSize:    100,
		MaxBackups: 5,
		MaxAge:     30,
		Compress:   true,
	}

	config.Database = database.Config{
		Host:     "localhost",
		Port:     5432,
		User:     "postgres",
		Password: "postgres",
		DBName:   "clustereye",
		SSLMode:  "disable",
	}

	// InfluxDB varsayılan ayarları
	config.InfluxDB = InfluxDBConfig{
		URL:           "http://localhost:8086",
		Token:         "",
		Organization:  "clustereye",
		Bucket:        "clustereye",
		Enabled:       false, // Varsayılan olarak kapalı
		BatchSize:     1000,
		FlushInterval: 10,
	}

	// Konfigürasyonu YAML olarak dönüştür
	data, err := yaml.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("varsayılan konfigürasyon oluşturulamadı: %w", err)
	}

	// Dizin yoksa oluştur
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("konfigürasyon dizini oluşturulamadı: %w", err)
	}

	// Dosyayı yaz
	if err := ioutil.WriteFile(configPath, data, 0644); err != nil {
		return nil, fmt.Errorf("konfigürasyon dosyası yazılamadı: %w", err)
	}

	return config, nil
}

// createDefaultAgentConfig, varsayılan agent konfigürasyonunu oluşturur
func createDefaultAgentConfig(configPath string) (*AgentConfig, error) {
	// Varsayılan ayarlar
	config := &AgentConfig{}
	config.Key = "agent_key"
	config.Name = "Agent"
	config.GRPC.ServerAddress = "localhost:50051"

	// PostgreSQL varsayılan ayarları
	config.PostgreSQL.Cluster = "postgres"
	config.PostgreSQL.Port = "5432"

	// MongoDB varsayılan ayarları
	config.Mongo.Replset = "rs0"
	config.Mongo.Port = "27017"

	// Konfigürasyonu YAML olarak dönüştür
	data, err := yaml.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("varsayılan konfigürasyon oluşturulamadı: %w", err)
	}

	// Dizin yoksa oluştur
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("konfigürasyon dizini oluşturulamadı: %w", err)
	}

	// Dosyayı yaz
	if err := ioutil.WriteFile(configPath, data, 0644); err != nil {
		return nil, fmt.Errorf("konfigürasyon dosyası yazılamadı: %w", err)
	}

	return config, nil
}

// getConfigPath, konfigürasyon dosyasının tam yolunu döndürür
func getConfigPath(filename string) string {
	// Önce çalışma dizinine bakar, sonra /etc/clustereye dizinine bakar
	if _, err := os.Stat(filename); err == nil {
		return filename
	}

	// /etc/clustereye dizinine bak
	etcPath := filepath.Join("/etc", "clustereye", filename)
	if _, err := os.Stat(etcPath); err == nil {
		return etcPath
	}

	// Dosya bulunamadıysa, çalışma dizinine yaz
	return filename
}
