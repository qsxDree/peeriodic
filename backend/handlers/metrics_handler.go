package handlers

import (
	"net/http"
	"runtime"
	"time"

	"github.com/logoes0/peeriodic.git/middleware"
	"github.com/logoes0/peeriodic.git/services"
	"github.com/logoes0/peeriodic.git/utils"
)

// MetricsHandler exposes runtime operational metrics
type MetricsHandler struct {
	startTime time.Time
	wsService *services.WebSocketService
}

// NewMetricsHandler creates a new MetricsHandler
func NewMetricsHandler(wsService *services.WebSocketService) *MetricsHandler {
	return &MetricsHandler{
		startTime: time.Now(),
		wsService: wsService,
	}
}

// MetricsResponse holds system and application performance metrics
type MetricsResponse struct {
	UptimeSeconds    float64   `json:"uptime_seconds"`
	Timestamp        time.Time `json:"timestamp"`
	TotalRequests    uint64    `json:"total_requests"`
	TotalErrors      uint64    `json:"total_errors"`
	ActiveGoroutines int       `json:"active_goroutines"`
	MemoryAllocMB    float64   `json:"memory_alloc_mb"`
	MemorySysMB      float64   `json:"memory_sys_mb"`
	NumGC            uint32    `json:"num_gc"`
	ActiveRooms      int       `json:"active_rooms"`
	TotalClients     int       `json:"total_clients"`
}

// HandleMetrics returns operational performance metrics
func (mh *MetricsHandler) HandleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		utils.MethodNotAllowed(w)
		return
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	activeRooms := 0
	totalClients := 0
	var roomStats map[string]int

	if mh.wsService != nil {
		roomStats = mh.wsService.GetRoomStats()
		activeRooms = len(roomStats)
		for _, count := range roomStats {
			totalClients += count
		}
	}

	totalReq, totalErr := middleware.GetRequestMetrics()

	resp := MetricsResponse{
		UptimeSeconds:    time.Since(mh.startTime).Seconds(),
		Timestamp:        time.Now().UTC(),
		TotalRequests:    totalReq,
		TotalErrors:      totalErr,
		ActiveGoroutines: runtime.NumGoroutine(),
		MemoryAllocMB:    float64(mem.Alloc) / (1024 * 1024),
		MemorySysMB:      float64(mem.Sys) / (1024 * 1024),
		NumGC:            mem.NumGC,
		ActiveRooms:      activeRooms,
		TotalClients:     totalClients,
	}

	utils.JSONResponse(w, http.StatusOK, resp)
}
