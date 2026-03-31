package handlers

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"
)

type HealthStatus struct {
	Status     string `json:"status"`
	GoVersion  string `json:"go_version"`
	Goroutines int    `json:"goroutines"`
	Uptime     string `json:"uptime"`
}

var bootTime = time.Now()

func HealthCheck(w http.ResponseWriter, r *http.Request) {
	status := HealthStatus{
		Status:     "healthy",
		GoVersion:  runtime.Version(),
		Goroutines: runtime.NumGoroutine(),
		Uptime:     time.Since(bootTime).Round(time.Second).String(),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}
