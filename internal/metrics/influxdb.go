package metrics

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/sefaphlvn/clustereye-test/internal/config"
	"google.golang.org/protobuf/types/known/structpb"
)

// InfluxDBWriter, InfluxDB'ye metrik yazma işlemlerini yönetir
type InfluxDBWriter struct {
	client   influxdb2.Client
	writeAPI api.WriteAPIBlocking
	config   config.InfluxDBConfig
	enabled  bool
}

// NewInfluxDBWriter, yeni bir InfluxDB writer oluşturur
func NewInfluxDBWriter(cfg config.InfluxDBConfig) (*InfluxDBWriter, error) {
	if !cfg.Enabled {
		log.Printf("[INFO] InfluxDB devre dışı")
		return &InfluxDBWriter{enabled: false}, nil
	}

	// InfluxDB client oluştur
	client := influxdb2.NewClient(cfg.URL, cfg.Token)

	// Bağlantıyı test et
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	health, err := client.Health(ctx)
	if err != nil {
		return nil, fmt.Errorf("InfluxDB bağlantı hatası: %v", err)
	}

	if health.Status != "pass" {
		return nil, fmt.Errorf("InfluxDB sağlık kontrolü başarısız: %s", health.Status)
	}

	// Write API oluştur
	writeAPI := client.WriteAPIBlocking(cfg.Organization, cfg.Bucket)

	log.Printf("[INFO] InfluxDB bağlantısı başarılı - URL: %s, Org: %s, Bucket: %s",
		cfg.URL, cfg.Organization, cfg.Bucket)

	return &InfluxDBWriter{
		client:   client,
		writeAPI: writeAPI,
		config:   cfg,
		enabled:  true,
	}, nil
}

// WriteSystemMetrics, sistem metriklerini InfluxDB'ye yazar
func (w *InfluxDBWriter) WriteSystemMetrics(ctx context.Context, agentID string, metrics *structpb.Struct) error {
	if !w.enabled {
		return nil
	}

	timestamp := time.Now()
	points := make([]*write.Point, 0)

	// Metrics'i map'e dönüştür
	metricsMap := metrics.AsMap()

	// CPU metrikleri (nested format)
	if cpuData, ok := metricsMap["cpu"]; ok {
		if cpuMap, ok := cpuData.(map[string]interface{}); ok {
			points = append(points, w.createCPUPoints(agentID, cpuMap, timestamp)...)
		}
	}

	// Memory metrikleri (nested format)
	if memData, ok := metricsMap["memory"]; ok {
		if memMap, ok := memData.(map[string]interface{}); ok {
			points = append(points, w.createMemoryPoints(agentID, memMap, timestamp)...)
		}
	}

	// Disk metrikleri (nested format)
	if diskData, ok := metricsMap["disk"]; ok {
		if diskMap, ok := diskData.(map[string]interface{}); ok {
			points = append(points, w.createDiskPoints(agentID, diskMap, timestamp)...)
		}
	}

	// Network metrikleri (nested format)
	if netData, ok := metricsMap["network"]; ok {
		if netMap, ok := netData.(map[string]interface{}); ok {
			points = append(points, w.createNetworkPoints(agentID, netMap, timestamp)...)
		}
	}

	// Database metrikleri (nested format)
	if dbData, ok := metricsMap["database"]; ok {
		if dbMap, ok := dbData.(map[string]interface{}); ok {
			points = append(points, w.createDatabasePoints(agentID, dbMap, timestamp)...)
		}
	}

	// MSSQL flat format desteği (agent'ınızın gönderdiği format)
	points = append(points, w.createFlatFormatPoints(agentID, metricsMap, timestamp)...)

	// Tüm point'leri yaz
	if len(points) > 0 {
		err := w.writeAPI.WritePoint(ctx, points...)
		if err != nil {
			return fmt.Errorf("InfluxDB yazma hatası: %v", err)
		}
		log.Printf("[DEBUG] InfluxDB'ye %d metrik yazıldı - Agent: %s", len(points), agentID)
	}

	return nil
}

// createCPUPoints, CPU metriklerini InfluxDB point'lerine dönüştürür
func (w *InfluxDBWriter) createCPUPoints(agentID string, cpuData map[string]interface{}, timestamp time.Time) []*write.Point {
	points := make([]*write.Point, 0)

	// CPU kullanım yüzdesi
	if usage, ok := w.getFloatValue(cpuData, "usage_percent"); ok {
		point := influxdb2.NewPointWithMeasurement("cpu_usage").
			AddTag("agent_id", agentID).
			AddField("usage_percent", usage).
			SetTime(timestamp)
		points = append(points, point)
	}

	// CPU core sayısı
	if cores, ok := w.getFloatValue(cpuData, "cores"); ok {
		point := influxdb2.NewPointWithMeasurement("cpu_info").
			AddTag("agent_id", agentID).
			AddField("cores", cores).
			SetTime(timestamp)
		points = append(points, point)
	}

	// Load average
	if loadAvg, ok := cpuData["load_average"].(map[string]interface{}); ok {
		if load1, ok := w.getFloatValue(loadAvg, "1m"); ok {
			point := influxdb2.NewPointWithMeasurement("cpu_load").
				AddTag("agent_id", agentID).
				AddTag("period", "1m").
				AddField("load_average", load1).
				SetTime(timestamp)
			points = append(points, point)
		}
		if load5, ok := w.getFloatValue(loadAvg, "5m"); ok {
			point := influxdb2.NewPointWithMeasurement("cpu_load").
				AddTag("agent_id", agentID).
				AddTag("period", "5m").
				AddField("load_average", load5).
				SetTime(timestamp)
			points = append(points, point)
		}
		if load15, ok := w.getFloatValue(loadAvg, "15m"); ok {
			point := influxdb2.NewPointWithMeasurement("cpu_load").
				AddTag("agent_id", agentID).
				AddTag("period", "15m").
				AddField("load_average", load15).
				SetTime(timestamp)
			points = append(points, point)
		}
	}

	return points
}

// createMemoryPoints, Memory metriklerini InfluxDB point'lerine dönüştürür
func (w *InfluxDBWriter) createMemoryPoints(agentID string, memData map[string]interface{}, timestamp time.Time) []*write.Point {
	points := make([]*write.Point, 0)

	// Memory kullanım yüzdesi
	if usage, ok := w.getFloatValue(memData, "usage_percent"); ok {
		point := influxdb2.NewPointWithMeasurement("memory_usage").
			AddTag("agent_id", agentID).
			AddField("usage_percent", usage).
			SetTime(timestamp)
		points = append(points, point)
	}

	// Total memory
	if total, ok := w.getFloatValue(memData, "total_bytes"); ok {
		point := influxdb2.NewPointWithMeasurement("memory_info").
			AddTag("agent_id", agentID).
			AddField("total_bytes", total).
			SetTime(timestamp)
		points = append(points, point)
	}

	// Used memory
	if used, ok := w.getFloatValue(memData, "used_bytes"); ok {
		point := influxdb2.NewPointWithMeasurement("memory_usage").
			AddTag("agent_id", agentID).
			AddField("used_bytes", used).
			SetTime(timestamp)
		points = append(points, point)
	}

	// Available memory
	if available, ok := w.getFloatValue(memData, "available_bytes"); ok {
		point := influxdb2.NewPointWithMeasurement("memory_usage").
			AddTag("agent_id", agentID).
			AddField("available_bytes", available).
			SetTime(timestamp)
		points = append(points, point)
	}

	return points
}

// createDiskPoints, Disk metriklerini InfluxDB point'lerine dönüştürür
func (w *InfluxDBWriter) createDiskPoints(agentID string, diskData map[string]interface{}, timestamp time.Time) []*write.Point {
	points := make([]*write.Point, 0)

	// Disk kullanımları (array olarak gelebilir)
	if disks, ok := diskData["disks"].([]interface{}); ok {
		for _, diskInterface := range disks {
			if disk, ok := diskInterface.(map[string]interface{}); ok {
				mountpoint := w.getStringValue(disk, "mountpoint", "/")
				device := w.getStringValue(disk, "device", "unknown")

				// Disk kullanım yüzdesi
				if usage, ok := w.getFloatValue(disk, "usage_percent"); ok {
					point := influxdb2.NewPointWithMeasurement("disk_usage").
						AddTag("agent_id", agentID).
						AddTag("mountpoint", mountpoint).
						AddTag("device", device).
						AddField("usage_percent", usage).
						SetTime(timestamp)
					points = append(points, point)
				}

				// Total space
				if total, ok := w.getFloatValue(disk, "total_bytes"); ok {
					point := influxdb2.NewPointWithMeasurement("disk_info").
						AddTag("agent_id", agentID).
						AddTag("mountpoint", mountpoint).
						AddTag("device", device).
						AddField("total_bytes", total).
						SetTime(timestamp)
					points = append(points, point)
				}

				// Used space
				if used, ok := w.getFloatValue(disk, "used_bytes"); ok {
					point := influxdb2.NewPointWithMeasurement("disk_usage").
						AddTag("agent_id", agentID).
						AddTag("mountpoint", mountpoint).
						AddTag("device", device).
						AddField("used_bytes", used).
						SetTime(timestamp)
					points = append(points, point)
				}
			}
		}
	}

	return points
}

// createNetworkPoints, Network metriklerini InfluxDB point'lerine dönüştürür
func (w *InfluxDBWriter) createNetworkPoints(agentID string, netData map[string]interface{}, timestamp time.Time) []*write.Point {
	points := make([]*write.Point, 0)

	// Network interfaces (array olarak gelebilir)
	if interfaces, ok := netData["interfaces"].([]interface{}); ok {
		for _, ifaceInterface := range interfaces {
			if iface, ok := ifaceInterface.(map[string]interface{}); ok {
				ifaceName := w.getStringValue(iface, "name", "unknown")

				// Bytes sent
				if bytesSent, ok := w.getFloatValue(iface, "bytes_sent"); ok {
					point := influxdb2.NewPointWithMeasurement("network_io").
						AddTag("agent_id", agentID).
						AddTag("interface", ifaceName).
						AddTag("direction", "sent").
						AddField("bytes", bytesSent).
						SetTime(timestamp)
					points = append(points, point)
				}

				// Bytes received
				if bytesRecv, ok := w.getFloatValue(iface, "bytes_recv"); ok {
					point := influxdb2.NewPointWithMeasurement("network_io").
						AddTag("agent_id", agentID).
						AddTag("interface", ifaceName).
						AddTag("direction", "received").
						AddField("bytes", bytesRecv).
						SetTime(timestamp)
					points = append(points, point)
				}

				// Packets sent
				if packetsSent, ok := w.getFloatValue(iface, "packets_sent"); ok {
					point := influxdb2.NewPointWithMeasurement("network_packets").
						AddTag("agent_id", agentID).
						AddTag("interface", ifaceName).
						AddTag("direction", "sent").
						AddField("packets", packetsSent).
						SetTime(timestamp)
					points = append(points, point)
				}

				// Packets received
				if packetsRecv, ok := w.getFloatValue(iface, "packets_recv"); ok {
					point := influxdb2.NewPointWithMeasurement("network_packets").
						AddTag("agent_id", agentID).
						AddTag("interface", ifaceName).
						AddTag("direction", "received").
						AddField("packets", packetsRecv).
						SetTime(timestamp)
					points = append(points, point)
				}
			}
		}
	}

	return points
}

// createDatabasePoints, Database metriklerini InfluxDB point'lerine dönüştürür
func (w *InfluxDBWriter) createDatabasePoints(agentID string, dbData map[string]interface{}, timestamp time.Time) []*write.Point {
	points := make([]*write.Point, 0)

	// PostgreSQL metrikleri
	if pgData, ok := dbData["postgresql"].(map[string]interface{}); ok {
		points = append(points, w.createPostgreSQLPoints(agentID, pgData, timestamp)...)
	}

	// MongoDB metrikleri
	if mongoData, ok := dbData["mongodb"].(map[string]interface{}); ok {
		points = append(points, w.createMongoDBPoints(agentID, mongoData, timestamp)...)
	}

	// MSSQL metrikleri
	if mssqlData, ok := dbData["mssql"].(map[string]interface{}); ok {
		points = append(points, w.createMSSQLPoints(agentID, mssqlData, timestamp)...)
	}

	return points
}

// createPostgreSQLPoints, PostgreSQL metriklerini oluşturur
func (w *InfluxDBWriter) createPostgreSQLPoints(agentID string, pgData map[string]interface{}, timestamp time.Time) []*write.Point {
	points := make([]*write.Point, 0)

	// Connection count
	if connections, ok := w.getFloatValue(pgData, "connections"); ok {
		point := influxdb2.NewPointWithMeasurement("postgresql_connections").
			AddTag("agent_id", agentID).
			AddField("count", connections).
			SetTime(timestamp)
		points = append(points, point)
	}

	// Database size
	if dbSize, ok := w.getFloatValue(pgData, "database_size_bytes"); ok {
		point := influxdb2.NewPointWithMeasurement("postgresql_database").
			AddTag("agent_id", agentID).
			AddField("size_bytes", dbSize).
			SetTime(timestamp)
		points = append(points, point)
	}

	// Transactions per second
	if tps, ok := w.getFloatValue(pgData, "transactions_per_second"); ok {
		point := influxdb2.NewPointWithMeasurement("postgresql_performance").
			AddTag("agent_id", agentID).
			AddField("transactions_per_second", tps).
			SetTime(timestamp)
		points = append(points, point)
	}

	return points
}

// createMongoDBPoints, MongoDB metriklerini oluşturur
func (w *InfluxDBWriter) createMongoDBPoints(agentID string, mongoData map[string]interface{}, timestamp time.Time) []*write.Point {
	points := make([]*write.Point, 0)

	// Connection count
	if connections, ok := w.getFloatValue(mongoData, "connections"); ok {
		point := influxdb2.NewPointWithMeasurement("mongodb_connections").
			AddTag("agent_id", agentID).
			AddField("count", connections).
			SetTime(timestamp)
		points = append(points, point)
	}

	// Operations per second
	if ops, ok := w.getFloatValue(mongoData, "operations_per_second"); ok {
		point := influxdb2.NewPointWithMeasurement("mongodb_performance").
			AddTag("agent_id", agentID).
			AddField("operations_per_second", ops).
			SetTime(timestamp)
		points = append(points, point)
	}

	// Memory usage
	if memUsage, ok := w.getFloatValue(mongoData, "memory_usage_bytes"); ok {
		point := influxdb2.NewPointWithMeasurement("mongodb_memory").
			AddTag("agent_id", agentID).
			AddField("usage_bytes", memUsage).
			SetTime(timestamp)
		points = append(points, point)
	}

	return points
}

// createMSSQLPoints, MSSQL metriklerini oluşturur
func (w *InfluxDBWriter) createMSSQLPoints(agentID string, mssqlData map[string]interface{}, timestamp time.Time) []*write.Point {
	points := make([]*write.Point, 0)

	// Connection count
	if connections, ok := w.getFloatValue(mssqlData, "connections"); ok {
		point := influxdb2.NewPointWithMeasurement("mssql_connections").
			AddTag("agent_id", agentID).
			AddField("count", connections).
			SetTime(timestamp)
		points = append(points, point)
	}

	// CPU usage
	if cpuUsage, ok := w.getFloatValue(mssqlData, "cpu_usage_percent"); ok {
		point := influxdb2.NewPointWithMeasurement("mssql_performance").
			AddTag("agent_id", agentID).
			AddField("cpu_usage_percent", cpuUsage).
			SetTime(timestamp)
		points = append(points, point)
	}

	// Buffer cache hit ratio
	if bufferHit, ok := w.getFloatValue(mssqlData, "buffer_cache_hit_ratio"); ok {
		point := influxdb2.NewPointWithMeasurement("mssql_performance").
			AddTag("agent_id", agentID).
			AddField("buffer_cache_hit_ratio", bufferHit).
			SetTime(timestamp)
		points = append(points, point)
	}

	return points
}

// createFlatFormatPoints, düz (flat) formattaki metrikleri InfluxDB point'lerine dönüştürür
// MSSQL agent'ının gönderdiği format için
func (w *InfluxDBWriter) createFlatFormatPoints(agentID string, metricsMap map[string]interface{}, timestamp time.Time) []*write.Point {
	points := make([]*write.Point, 0)

	// CPU metrikleri (flat format)
	if cpuUsage, ok := w.getFloatValue(metricsMap, "cpu_usage"); ok {
		point := influxdb2.NewPointWithMeasurement("cpu_usage").
			AddTag("agent_id", agentID).
			AddField("usage_percent", cpuUsage).
			SetTime(timestamp)
		points = append(points, point)
	}

	if cpuCores, ok := w.getFloatValue(metricsMap, "cpu_cores"); ok {
		point := influxdb2.NewPointWithMeasurement("cpu_info").
			AddTag("agent_id", agentID).
			AddField("cores", cpuCores).
			SetTime(timestamp)
		points = append(points, point)
	}

	// Load average metrikleri
	if load1m, ok := w.getFloatValue(metricsMap, "load_average_1m"); ok {
		point := influxdb2.NewPointWithMeasurement("cpu_load").
			AddTag("agent_id", agentID).
			AddTag("period", "1m").
			AddField("load_average", load1m).
			SetTime(timestamp)
		points = append(points, point)
	}

	if load5m, ok := w.getFloatValue(metricsMap, "load_average_5m"); ok {
		point := influxdb2.NewPointWithMeasurement("cpu_load").
			AddTag("agent_id", agentID).
			AddTag("period", "5m").
			AddField("load_average", load5m).
			SetTime(timestamp)
		points = append(points, point)
	}

	if load15m, ok := w.getFloatValue(metricsMap, "load_average_15m"); ok {
		point := influxdb2.NewPointWithMeasurement("cpu_load").
			AddTag("agent_id", agentID).
			AddTag("period", "15m").
			AddField("load_average", load15m).
			SetTime(timestamp)
		points = append(points, point)
	}

	// Memory metrikleri (flat format)
	if memUsage, ok := w.getFloatValue(metricsMap, "memory_usage"); ok {
		point := influxdb2.NewPointWithMeasurement("memory_usage").
			AddTag("agent_id", agentID).
			AddField("usage_percent", memUsage).
			SetTime(timestamp)
		points = append(points, point)
	}

	if totalMem, ok := w.getFloatValue(metricsMap, "total_memory"); ok {
		point := influxdb2.NewPointWithMeasurement("memory_info").
			AddTag("agent_id", agentID).
			AddField("total_bytes", totalMem).
			SetTime(timestamp)
		points = append(points, point)
	}

	if freeMem, ok := w.getFloatValue(metricsMap, "free_memory"); ok {
		point := influxdb2.NewPointWithMeasurement("memory_usage").
			AddTag("agent_id", agentID).
			AddField("available_bytes", freeMem).
			SetTime(timestamp)
		points = append(points, point)

		// Used memory hesapla
		if totalMem, ok := w.getFloatValue(metricsMap, "total_memory"); ok {
			usedMem := totalMem - freeMem
			point := influxdb2.NewPointWithMeasurement("memory_usage").
				AddTag("agent_id", agentID).
				AddField("used_bytes", usedMem).
				SetTime(timestamp)
			points = append(points, point)
		}
	}

	// Disk metrikleri (flat format)
	if totalDisk, ok := w.getFloatValue(metricsMap, "total_disk"); ok {
		point := influxdb2.NewPointWithMeasurement("disk_info").
			AddTag("agent_id", agentID).
			AddTag("mountpoint", "/").
			AddTag("device", "system").
			AddField("total_bytes", totalDisk).
			SetTime(timestamp)
		points = append(points, point)
	}

	if freeDisk, ok := w.getFloatValue(metricsMap, "free_disk"); ok {
		point := influxdb2.NewPointWithMeasurement("disk_usage").
			AddTag("agent_id", agentID).
			AddTag("mountpoint", "/").
			AddTag("device", "system").
			AddField("available_bytes", freeDisk).
			SetTime(timestamp)
		points = append(points, point)

		// Used disk ve usage percent hesapla
		if totalDisk, ok := w.getFloatValue(metricsMap, "total_disk"); ok {
			usedDisk := totalDisk - freeDisk
			point := influxdb2.NewPointWithMeasurement("disk_usage").
				AddTag("agent_id", agentID).
				AddTag("mountpoint", "/").
				AddTag("device", "system").
				AddField("used_bytes", usedDisk).
				SetTime(timestamp)
			points = append(points, point)

			// Usage percentage
			if totalDisk > 0 {
				usagePercent := (usedDisk / totalDisk) * 100
				point := influxdb2.NewPointWithMeasurement("disk_usage").
					AddTag("agent_id", agentID).
					AddTag("mountpoint", "/").
					AddTag("device", "system").
					AddField("usage_percent", usagePercent).
					SetTime(timestamp)
				points = append(points, point)
			}
		}
	}

	// System info metrikleri
	if osVersion, ok := metricsMap["os_version"].(string); ok && osVersion != "" {
		point := influxdb2.NewPointWithMeasurement("system_info").
			AddTag("agent_id", agentID).
			AddField("os_version", osVersion).
			SetTime(timestamp)
		points = append(points, point)
	}

	if kernelVersion, ok := metricsMap["kernel_version"].(string); ok && kernelVersion != "" {
		point := influxdb2.NewPointWithMeasurement("system_info").
			AddTag("agent_id", agentID).
			AddField("kernel_version", kernelVersion).
			SetTime(timestamp)
		points = append(points, point)
	}

	if uptime, ok := w.getFloatValue(metricsMap, "uptime"); ok {
		point := influxdb2.NewPointWithMeasurement("system_info").
			AddTag("agent_id", agentID).
			AddField("uptime_seconds", uptime).
			SetTime(timestamp)
		points = append(points, point)
	}

	return points
}

// Helper functions

// getFloatValue, interface{}'den float64 değer çıkarır
func (w *InfluxDBWriter) getFloatValue(data map[string]interface{}, key string) (float64, bool) {
	if val, ok := data[key]; ok {
		switch v := val.(type) {
		case float64:
			return v, true
		case float32:
			return float64(v), true
		case int:
			return float64(v), true
		case int64:
			return float64(v), true
		case string:
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return f, true
			}
		}
	}
	return 0, false
}

// getStringValue, interface{}'den string değer çıkarır
func (w *InfluxDBWriter) getStringValue(data map[string]interface{}, key, defaultValue string) string {
	if val, ok := data[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return defaultValue
}

// WriteMetric, tek bir metric'i InfluxDB'ye yazar
func (w *InfluxDBWriter) WriteMetric(ctx context.Context, measurement string, tags map[string]string, fields map[string]interface{}, timestamp time.Time) error {
	if !w.enabled {
		return nil
	}

	// Point oluştur
	point := influxdb2.NewPointWithMeasurement(measurement)

	// Tags ekle
	for k, v := range tags {
		point = point.AddTag(k, v)
	}

	// Fields ekle
	for k, v := range fields {
		point = point.AddField(k, v)
	}

	// Timestamp ayarla
	point = point.SetTime(timestamp)

	// InfluxDB'ye yaz
	err := w.writeAPI.WritePoint(ctx, point)
	if err != nil {
		return fmt.Errorf("InfluxDB yazma hatası: %v", err)
	}

	return nil
}

// QueryMetrics, InfluxDB'den metrikleri sorgular
func (w *InfluxDBWriter) QueryMetrics(ctx context.Context, query string) ([]map[string]interface{}, error) {
	if !w.enabled {
		return nil, fmt.Errorf("InfluxDB devre dışı")
	}

	queryAPI := w.client.QueryAPI(w.config.Organization)
	result, err := queryAPI.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("InfluxDB sorgu hatası: %v", err)
	}
	defer result.Close()

	var records []map[string]interface{}
	for result.Next() {
		record := make(map[string]interface{})
		for key, value := range result.Record().Values() {
			record[key] = value
		}
		records = append(records, record)
	}

	if result.Err() != nil {
		return nil, fmt.Errorf("InfluxDB sonuç hatası: %v", result.Err())
	}

	return records, nil
}

// Close, InfluxDB bağlantısını kapatır
func (w *InfluxDBWriter) Close() {
	if w.enabled && w.client != nil {
		w.client.Close()
		log.Printf("[INFO] InfluxDB bağlantısı kapatıldı")
	}
}
