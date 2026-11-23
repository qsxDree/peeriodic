package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/logoes0/peeriodic.git/services"
	"github.com/logoes0/peeriodic.git/utils"
)

// HealthHandler manages liveness and readiness probes
type HealthHandler struct {
	startTime time.Time
	dbService *services.DatabaseService
}

// NewHealthHandler creates a new HealthHandler
func NewHealthHandler(dbService *services.DatabaseService) *HealthHandler {
	return &HealthHandler{
		startTime: time.Now(),
		dbService: dbService,
	}
}

// HealthResponse represents liveness health status
type HealthResponse struct {
	Status        string    `json:"status"`
	UptimeSeconds float64   `json:"uptime_seconds"`
	Timestamp     time.Time `json:"timestamp"`
}

// ReadyResponse represents readiness probe status
type ReadyResponse struct {
	Status   string `json:"status"`
	Database string `json:"database"`
	Error    string `json:"error,omitempty"`
}

// HandleHealthz responds to Kubernetes / container liveness probes
func (h *HealthHandler) HandleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		utils.MethodNotAllowed(w)
		return
	}

	uptime := time.Since(h.startTime).Seconds()
	utils.JSONResponse(w, http.StatusOK, HealthResponse{
		Status:        "ok",
		UptimeSeconds: uptime,
		Timestamp:     time.Now().UTC(),
	})
}

// HandleReadyz responds to Kubernetes / container readiness probes
func (h *HealthHandler) HandleReadyz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		utils.MethodNotAllowed(w)
		return
	}

	if h.dbService == nil {
		utils.JSONResponse(w, http.StatusServiceUnavailable, ReadyResponse{
			Status:   "unavailable",
			Database: "uninitialized",
			Error:    "database service is not configured",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := h.dbService.Ping(ctx); err != nil {
		utils.JSONResponse(w, http.StatusServiceUnavailable, ReadyResponse{
			Status:   "unavailable",
			Database: "disconnected",
			Error:    err.Error(),
		})
		return
	}

	utils.JSONResponse(w, http.StatusOK, ReadyResponse{
		Status:   "ready",
		Database: "connected",
	})
}
