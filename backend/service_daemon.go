package main

import (
	"net/http"
	"strings"
	"time"
)

// DaemonService — доступ к vlesssubtest (health-статус).
type DaemonService struct {
	baseURL string
}

func NewDaemonService(baseURL string) *DaemonService {
	return &DaemonService{baseURL: baseURL}
}

// Status возвращает статус демона (GET /test).
func (d *DaemonService) Status() VlessSubTestStatus {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(strings.TrimRight(d.baseURL, "/") + "/test")
	if err != nil {
		return VlessSubTestStatus{Available: false, DaemonURL: d.baseURL, Error: err.Error()}
	}
	resp.Body.Close()
	return VlessSubTestStatus{Available: true, DaemonURL: d.baseURL}
}
