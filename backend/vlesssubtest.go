package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"vlesspanel/dto"
)

// vlessSubTestClient — HTTP-клиент к демону vlesssubtest.
type vlessSubTestClient struct {
	baseURL string
}

func NewVlessSubTestClient(baseURL string) *vlessSubTestClient {
	return &vlessSubTestClient{baseURL: strings.TrimRight(baseURL, "/")}
}

// Status возвращает статус демона (GET /test).
func (c *vlessSubTestClient) Status() dto.VlessSubTestStatus {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(c.baseURL + "/test")
	if err != nil {
		return dto.VlessSubTestStatus{Available: false, DaemonURL: c.baseURL, Error: err.Error()}
	}
	resp.Body.Close()
	return dto.VlessSubTestStatus{Available: true, DaemonURL: c.baseURL}
}

// TestSingle прогоняет один vless-ключ через демон (POST /test-single).
// Ошибки сети/разбора оборачиваются sentinel'ами для различения на уровне сервисов.
func (c *vlessSubTestClient) TestSingle(vless string, timeout int) (dto.TestSingleResponse, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	reqBody, _ := json.Marshal(dto.TestSingleRequest{Vless: vless, Timeout: timeout})

	resp, err := client.Post(c.baseURL+"/test-single", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return dto.TestSingleResponse{}, fmt.Errorf("%w: %v", ErrDaemonUnreachable, err)
	}

	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	var single dto.TestSingleResponse
	if err := json.Unmarshal(body, &single); err != nil {
		return dto.TestSingleResponse{}, fmt.Errorf("%w: %v", ErrDaemonParse, err)
	}
	return single, nil
}
