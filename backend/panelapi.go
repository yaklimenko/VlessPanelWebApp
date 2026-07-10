package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// PanelAPI handles communication with 3X-UI panels
type PanelAPI struct {
	client *http.Client
}

// NewPanelAPI creates a new PanelAPI
func NewPanelAPI() *PanelAPI {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	return &PanelAPI{
		client: &http.Client{Transport: tr},
	}
}

// doRequest performs an HTTP request to a 3X-UI panel
func (api *PanelAPI) doRequest(method, url, token string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return api.client.Do(req)
}

// parseResponse parses a 3X-UI API response
func (api *PanelAPI) parseResponse(resp *http.Response) (*XUIResponse, error) {
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("3X-UI HTTP %d: %s", resp.StatusCode, string(data))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data))
	}

	var xuiResp XUIResponse
	if err := json.Unmarshal(data, &xuiResp); err != nil {
		log.Printf("parseResponse: failed to unmarshal 3X-UI response: %v\nBody: %s", err, string(data))
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	return &xuiResp, nil
}

// ListInbounds fetches all inbounds from a panel
func (api *PanelAPI) ListInbounds(panel Panel) ([]XUIInbound, error) {
	url := fmt.Sprintf("%s/panel/api/inbounds/list", strings.TrimRight(panel.URL, "/"))

	resp, err := api.doRequest("GET", url, panel.Token, nil)
	if err != nil {
		return nil, fmt.Errorf("listing inbounds: %w", err)
	}

	xuiResp, err := api.parseResponse(resp)
	if err != nil {
		return nil, err
	}

	if !xuiResp.Success {
		return nil, fmt.Errorf("3X-UI error: %s", xuiResp.Msg)
	}

	// Parse the obj field into []XUIInbound
	objBytes, err := json.Marshal(xuiResp.Obj)
	if err != nil {
		return nil, fmt.Errorf("marshaling obj: %w", err)
	}

	var inbounds []XUIInbound
	if err := json.Unmarshal(objBytes, &inbounds); err != nil {
		log.Printf("ListInbounds: failed to unmarshal inbounds array: %v\nObj JSON: %s", err, string(objBytes))
		return nil, fmt.Errorf("parsing inbounds: %w", err)
	}

	return inbounds, nil
}

// ListClients extracts all clients from all inbounds
func (api *PanelAPI) ListClients(panel Panel) ([]Client, error) {
	inbounds, err := api.ListInbounds(panel)
	if err != nil {
		return nil, err
	}

	clientMap := make(map[string]*Client)

	for _, inbound := range inbounds {
		if inbound.ClientStats == nil {
			continue
		}

			// Parse settings — 3X-UI v3.4.2+ returns settings as JSON object (not string)
		type ParsedSettings struct {
			Clients []struct {
				ID    string `json:"id"`
				Email string `json:"email"`
				Flow  string `json:"flow,omitempty"`
			} `json:"clients"`
		}

		var settings ParsedSettings
		if err := json.Unmarshal(inbound.Settings, &settings); err != nil {
			log.Printf("ListClients: failed to unmarshal settings for inbound %d (%s): %v\nSettings: %s",
				inbound.ID, inbound.Remark, err, string(inbound.Settings))
			continue
		}

		for _, stat := range inbound.ClientStats {
			clientID := stat.Email
			if clientID == "" {
				continue
			}

			if existing, ok := clientMap[clientID]; ok {
				// Add inbound to existing client
				existing.Inbounds = append(existing.Inbounds, inbound.Remark)
				existing.InboundIDs = append(existing.InboundIDs, inbound.ID)
			} else {
				client := Client{
					ID:         clientID,
					Email:      stat.Email,
					Enable:     stat.Enable,
					InboundIDs: []int{inbound.ID},
					Inbounds:   []string{inbound.Remark},
					Keys:       []VLESSKey{},
				}
				clientMap[clientID] = &client
			}
		}
	}

	clients := make([]Client, 0, len(clientMap))
	for _, c := range clientMap {
		clients = append(clients, *c)
	}

	return clients, nil
}

// GetClientKeys fetches VLESS keys for a specific client across all inbounds
func (api *PanelAPI) GetClientKeys(panel Panel, email string) ([]VLESSKey, error) {
	inbounds, err := api.ListInbounds(panel)
	if err != nil {
		return nil, err
	}

	var keys []VLESSKey

	for _, inbound := range inbounds {
		// Parse settings — 3X-UI v3.4.2+ returns settings as JSON object
		type ParsedSettings struct {
			Clients []struct {
				ID    string `json:"id"`
				Email string `json:"email"`
				Flow  string `json:"flow,omitempty"`
			} `json:"clients"`
		}

		var settings ParsedSettings
		if err := json.Unmarshal(inbound.Settings, &settings); err != nil {
			log.Printf("GetClientKeys: failed to unmarshal settings for inbound %d (%s) while getting keys for %s: %v\nSettings: %s",
				inbound.ID, inbound.Remark, email, err, string(inbound.Settings))
			continue
		}

		for _, client := range settings.Clients {
			if client.Email != email {
				continue
			}

			// Build VLESS key
			link := fmt.Sprintf("vless://%s@%s:%d?encryption=none&security=%s&type=%s#%s-%s",
				client.ID,
				// Extract host from panel URL
				extractHost(panel.URL),
				inbound.Port,
				guessSecurity(inbound.Protocol),
				guessTransport(inbound.Protocol),
				inbound.Remark,
				email,
			)

			keys = append(keys, VLESSKey{
				Label:     fmt.Sprintf("%s-%s", inbound.Remark, email),
				Protocol:  inbound.Protocol,
				Link:      link,
				Inbound:   inbound.Remark,
				Server:    extractHost(panel.URL),
				Port:      inbound.Port,
				Security:  guessSecurity(inbound.Protocol),
				Transport: guessTransport(inbound.Protocol),
			})
		}
	}

	return keys, nil
}

// CreateClient creates a new client on a panel's inbound
func (api *PanelAPI) CreateClient(panel Panel, inboundID int, email string) error {
	url := fmt.Sprintf("%s/panel/api/inbounds/addClient", strings.TrimRight(panel.URL, "/"))

	// Generate a UUID for the client
	uuid := "00000000-0000-0000-0000-" + strings.ReplaceAll(email, "@", "")

	// First, get the current inbound settings
	inbounds, err := api.ListInbounds(panel)
	if err != nil {
		return fmt.Errorf("getting inbounds: %w", err)
	}

	var targetInbound *XUIInbound
	for i, ib := range inbounds {
		if ib.ID == inboundID {
			targetInbound = &inbounds[i]
			break
		}
	}

	if targetInbound == nil {
		return fmt.Errorf("inbound %d not found", inboundID)
	}

	// Parse current settings — 3X-UI v3.4.2+ returns settings as JSON object
	type ParsedSettings struct {
		Clients []map[string]interface{} `json:"clients"`
	}

	var settings ParsedSettings
	if err := json.Unmarshal(targetInbound.Settings, &settings); err != nil {
		log.Printf("CreateClient: failed to unmarshal settings for inbound %d (%s): %v\nSettings: %s",
			targetInbound.ID, targetInbound.Remark, err, string(targetInbound.Settings))
		return fmt.Errorf("parsing inbound settings: %w", err)
	}

	// Create new client
	newClient := map[string]interface{}{
		"id":    uuid,
		"email": email,
		"flow":  "xtls-rprx-vision",
	}

	if settings.Clients == nil {
		settings.Clients = make([]map[string]interface{}, 0)
	}
	settings.Clients = append(settings.Clients, newClient)

	updatedSettings, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshaling updated settings: %w", err)
	}

	payload := map[string]interface{}{
		"id":       inboundID,
		"settings": string(updatedSettings),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling payload: %w", err)
	}

	resp, err := api.doRequest("POST", url, panel.Token, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	xuiResp, err := api.parseResponse(resp)
	if err != nil {
		return err
	}

	if !xuiResp.Success {
		return fmt.Errorf("3X-UI error: %s", xuiResp.Msg)
	}

	return nil
}

// extractHost extracts the host from a panel URL
func extractHost(url string) string {
	// Remove protocol
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")
	// Remove port and path
	if idx := strings.Index(url, ":"); idx > 0 {
		url = url[:idx]
	}
	if idx := strings.Index(url, "/"); idx > 0 {
		url = url[:idx]
	}
	return url
}

// guessSecurity guesses the security type based on protocol
func guessSecurity(protocol string) string {
	switch strings.ToLower(protocol) {
	case "vless":
		return "reality"
	default:
		return "none"
	}
}

// guessTransport guesses the transport type based on protocol
func guessTransport(protocol string) string {
	return "tcp"
}
