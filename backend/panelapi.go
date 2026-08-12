package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// PanelAPI handles communication with 3X-UI panels
type PanelAPI struct {
	client *http.Client
}

// NewPanelAPI creates a new PanelAPI.
// The timeout is 10 seconds per operation with 3X-UI (per spec).
func NewPanelAPI() *PanelAPI {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	return &PanelAPI{
		client: &http.Client{
			Transport: tr,
			Timeout:   10 * time.Second,
		},
	}
}

// buildURL constructs the full API URL for a panel endpoint
func (api *PanelAPI) buildURL(panel Panel, path string) string {
	base := strings.TrimRight(panel.URL, "/")
	if panel.WebBasePath != "" {
		base = strings.TrimRight(base+"/"+strings.Trim(panel.WebBasePath, "/"), "/")
	}
	return fmt.Sprintf("%s/%s", base, strings.TrimLeft(path, "/"))
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
	url := api.buildURL(panel, "panel/api/inbounds/list")

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

// fetchXUIClients fetches raw clients from /panel/api/clients/list (3X-UI v3.4.2+)
func (api *PanelAPI) fetchXUIClients(panel Panel) ([]XUIClient, error) {
	url := api.buildURL(panel, "panel/api/clients/list")

	resp, err := api.doRequest("GET", url, panel.Token, nil)
	if err != nil {
		return nil, fmt.Errorf("listing clients (direct): %w", err)
	}

	xuiResp, err := api.parseResponse(resp)
	if err != nil {
		return nil, err
	}

	if !xuiResp.Success {
		return nil, fmt.Errorf("3X-UI error: %s", xuiResp.Msg)
	}

	objBytes, err := json.Marshal(xuiResp.Obj)
	if err != nil {
		return nil, fmt.Errorf("marshaling obj: %w", err)
	}

	var xuiClients []XUIClient
	if err := json.Unmarshal(objBytes, &xuiClients); err != nil {
		log.Printf("fetchXUIClients: failed to unmarshal clients: %v\nObj: %s", err, string(objBytes))
		return nil, fmt.Errorf("parsing clients: %w", err)
	}

	return xuiClients, nil
}

// mapClientsToInbounds converts raw XUI clients into Client, resolving inbound
// IDs to remark names via the inbounds list.
func mapClientsToInbounds(xuiClients []XUIClient, inbounds []XUIInbound) []Client {
	remarkMap := make(map[int]string, len(inbounds))
	for _, ib := range inbounds {
		remarkMap[ib.ID] = ib.Remark
	}

	clients := make([]Client, 0, len(xuiClients))
	for _, xc := range xuiClients {
		if xc.Email == "" {
			continue
		}
		inboundRemarks := make([]string, 0, len(xc.InboundIDs))
		for _, id := range xc.InboundIDs {
			if remark, ok := remarkMap[id]; ok {
				inboundRemarks = append(inboundRemarks, remark)
			}
		}
		clients = append(clients, Client{
			ID:         xc.Email,
			Email:      xc.Email,
			Enable:     xc.Enable,
			ExpiryTime: xc.ExpiryTime,
			InboundIDs: xc.InboundIDs,
			Inbounds:   inboundRemarks,
			Keys:       []VLESSKey{},
			Up:         xc.Traffic.Up,
			Down:       xc.Traffic.Down,
		})
	}

	return clients
}

// ListClientsDirect uses the dedicated /panel/api/clients/list endpoint (3X-UI v3.4.2+)
func (api *PanelAPI) ListClientsDirect(panel Panel) ([]Client, error) {
	xuiClients, err := api.fetchXUIClients(panel)
	if err != nil {
		return nil, err
	}

	// Fetch inbounds to map inbound IDs → remark names
	inbounds, err := api.ListInbounds(panel)
	if err != nil {
		log.Printf("ListClientsDirect: ListInbounds failed, inbound names unavailable: %v", err)
	}

	return mapClientsToInbounds(xuiClients, inbounds), nil
}

// ListClientsAndInbounds fetches clients and inbounds in parallel (one panel
// snapshot). Falls back to inbounds-based client extraction when the direct
// clients endpoint is unavailable; inbounds are required for status enrichment.
func (api *PanelAPI) ListClientsAndInbounds(panel Panel) ([]Client, []XUIInbound, error) {
	type res struct {
		kind     string // "clients" | "inbounds"
		clients  []XUIClient
		inbounds []XUIInbound
		err      error
	}
	ch := make(chan res, 2)
	go func() {
		clients, err := api.fetchXUIClients(panel)
		ch <- res{kind: "clients", clients: clients, err: err}
	}()
	go func() {
		inbounds, err := api.ListInbounds(panel)
		ch <- res{kind: "inbounds", inbounds: inbounds, err: err}
	}()

	var xuiClients []XUIClient
	var inbounds []XUIInbound
	var cerr, ierr error
	for i := 0; i < 2; i++ {
		r := <-ch
		if r.kind == "clients" {
			xuiClients, cerr = r.clients, r.err
		} else {
			inbounds, ierr = r.inbounds, r.err
		}
	}

	if ierr != nil {
		return nil, nil, ierr
	}
	if cerr != nil {
		log.Printf("ListClientsAndInbounds: direct clients failed, falling back to inbounds extraction: %v", cerr)
		return clientsFromInbounds(inbounds), inbounds, nil
	}

	return mapClientsToInbounds(xuiClients, inbounds), inbounds, nil
}

// ListClients extracts all clients, trying the direct endpoint first with inbounds fallback
func (api *PanelAPI) ListClients(panel Panel) ([]Client, error) {
	clients, err := api.ListClientsDirect(panel)
	if err == nil {
		return clients, nil
	}
	log.Printf("ListClientsDirect failed, falling back to inbounds-based method: %v", err)
	return api.listClientsFromInbounds(panel)
}

// listClientsFromInbounds extracts all clients from all inbounds (legacy method)
func (api *PanelAPI) listClientsFromInbounds(panel Panel) ([]Client, error) {
	inbounds, err := api.ListInbounds(panel)
	if err != nil {
		return nil, err
	}

	return clientsFromInbounds(inbounds), nil
}

// clientsFromInbounds extracts all clients from an already-fetched inbounds list.
func clientsFromInbounds(inbounds []XUIInbound) []Client {
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
				existing.Up += stat.Up
				existing.Down += stat.Down
			} else {
				client := Client{
					ID:         clientID,
					Email:      stat.Email,
					Enable:     stat.Enable,
					InboundIDs: []int{inbound.ID},
					Inbounds:   []string{inbound.Remark},
					Keys:       []VLESSKey{},
					Up:         stat.Up,
					Down:       stat.Down,
				}
				clientMap[clientID] = &client
			}
		}
	}

	clients := make([]Client, 0, len(clientMap))
	for _, c := range clientMap {
		clients = append(clients, *c)
	}

	return clients
}

// GetClientLinks fetches the links the panel itself generates for a client
// (the same strings the panel UI's Copy URL button copies), keyed by inbound port.
func (api *PanelAPI) GetClientLinks(panel Panel, email string) (map[int]string, error) {
	_url := api.buildURL(panel, "panel/api/clients/links/"+url.PathEscape(email))

	resp, err := api.doRequest("GET", _url, panel.Token, nil)
	if err != nil {
		return nil, fmt.Errorf("fetching client links: %w", err)
	}

	xuiResp, err := api.parseResponse(resp)
	if err != nil {
		return nil, err
	}

	if !xuiResp.Success {
		return nil, fmt.Errorf("3X-UI error: %s", xuiResp.Msg)
	}

	objBytes, err := json.Marshal(xuiResp.Obj)
	if err != nil {
		return nil, fmt.Errorf("marshaling obj: %w", err)
	}

	var links []string
	if err := json.Unmarshal(objBytes, &links); err != nil {
		log.Printf("GetClientLinks: failed to unmarshal links array: %v\nObj JSON: %s", err, string(objBytes))
		return nil, fmt.Errorf("parsing client links: %w", err)
	}

	byPort := make(map[int]string, len(links))
	for _, l := range links {
		u, err := url.Parse(l)
		if err != nil {
			continue
		}
		port, err := strconv.Atoi(u.Port())
		if err != nil {
			continue
		}
		byPort[port] = l
	}

	return byPort, nil
}

// GetClientKeys fetches VLESS keys for a specific client across all inbounds
func (api *PanelAPI) GetClientKeys(panel Panel, email string) ([]VLESSKey, error) {
	inbounds, err := api.ListInbounds(panel)
	if err != nil {
		return nil, err
	}

	// Prefer the links generated by the panel itself (includes pqv, spx suffix,
	// and the panel-chosen sid/sni) so keys match the panel's Copy URL output.
	panelLinks, linkErr := api.GetClientLinks(panel, email)
	if linkErr != nil {
		log.Printf("GetClientKeys: panel links endpoint unavailable for %s, falling back to locally generated links: %v", email, linkErr)
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

			host := extractHost(panel.URL)
			transport := guessTransport(inbound.Protocol)
			security := guessSecurity(inbound.Protocol)

			var params []string
			params = append(params, "encryption=none")

			if client.Flow != "" {
				params = append(params, "flow="+client.Flow)
			}

			ss := inbound.StreamSettings
			if ss != nil {
				if ss.Network != "" {
					transport = ss.Network
				}
				if ss.Security != "" {
					security = ss.Security
				}

				rs := ss.RealitySettings
				if rs != nil && rs.Settings != nil {
					rcs := rs.Settings
					if rcs.Fingerprint != "" {
						params = append(params, "fp="+rcs.Fingerprint)
					}
					if rcs.PublicKey != "" {
						params = append(params, "pbk="+rcs.PublicKey)
					}
				}
				if rs != nil {
					if len(rs.ShortIds) > 0 {
						params = append(params, "sid="+rs.ShortIds[0])
					}
					if rs.Settings != nil && rs.Settings.ServerName != "" {
						params = append(params, "sni="+rs.Settings.ServerName)
					} else if len(rs.ServerNames) > 0 {
						params = append(params, "sni="+rs.ServerNames[0])
					}
					if rs.Settings != nil && rs.Settings.SpiderX != "" {
						params = append(params, "spx="+url.QueryEscape(rs.Settings.SpiderX))
					}
					if rs.Settings != nil && rs.Settings.Mldsa65Verify != "" {
						params = append(params, "pqv="+rs.Settings.Mldsa65Verify)
					}
				}
			}

			params = append(params, "security="+security)
			params = append(params, "type="+transport)

			link := fmt.Sprintf("vless://%s@%s:%d?%s#%s-%s",
				client.ID, host, inbound.Port,
				strings.Join(params, "&"),
				inbound.Remark, email,
			)

			server := host
			if panelLink, ok := panelLinks[inbound.Port]; ok {
				link = panelLink
				if u, err := url.Parse(panelLink); err == nil && u.Hostname() != "" {
					server = u.Hostname()
				}
			}

			keys = append(keys, VLESSKey{
				Label:     fmt.Sprintf("%s-%s", inbound.Remark, email),
				Protocol:  inbound.Protocol,
				Link:      link,
				Inbound:   inbound.Remark,
				Server:    server,
				Port:      inbound.Port,
				Security:  security,
				Transport: transport,
			})
		}
	}

	return keys, nil
}

// CreateClient creates a new client on a panel's inbounds (3X-UI v3.5.0+)
func (api *PanelAPI) CreateClient(panel Panel, inboundID int, email string, expiryDate string) error {
	url := api.buildURL(panel, "panel/api/clients/add")

	clientObj := map[string]interface{}{
		"email":  email,
		"enable": true,
		"flow":   "xtls-rprx-vision",
	}

	if expiryDate != "" {
		t, err := time.Parse("2006-01-02", expiryDate)
		if err != nil {
			return fmt.Errorf("invalid expiryDate format (expected YYYY-MM-DD): %w", err)
		}
		expiryTime := t.Unix() * 1000
		clientObj["expiryTime"] = expiryTime
	}

	payload := map[string]interface{}{
		"client":     clientObj,
		"inboundIds": []int{inboundID},
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

// AttachClient attaches an existing client to an additional inbound.
//
// 3X-UI's attach API accepts only inboundIds and derives the client flow from
// the existing flow_override (EffectiveFlow — the first non-empty override
// among already attached inbounds). If the client was created on a
// non-flow-eligible inbound (e.g. XHTTP without vlessenc), flow_override is
// empty and the newly attached REALITY/TLS inbound gets NO flow — which breaks
// REALITY keys. So after a successful attach we push a targeted update with
// flow=xtls-rprx-vision: 3X-UI re-applies clientWithInboundFlow per inbound,
// keeping the flow on flow-eligible inbounds and stripping it on others.
func (api *PanelAPI) AttachClient(panel Panel, email string, inboundID int) error {
	_url := api.buildURL(panel, fmt.Sprintf("panel/api/clients/%s/attach", email))

	payload := map[string]interface{}{
		"inboundIds": []int{inboundID},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling payload: %w", err)
	}

	resp, err := api.doRequest("POST", _url, panel.Token, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("attaching client to inbound: %w", err)
	}

	xuiResp, err := api.parseResponse(resp)
	if err != nil {
		return err
	}

	if !xuiResp.Success {
		return fmt.Errorf("3X-UI error: %s", xuiResp.Msg)
	}

	// Push the XTLS Vision flow onto the just-attached inbound (no-op for
	// non-flow-eligible inbounds). Best-effort: if it fails, the attach itself
	// already succeeded, but we surface the error so callers know flow is off.
	if err := api.SetClientFlow(panel, email, inboundID); err != nil {
		return fmt.Errorf("client attached, but setting flow on inbound %d: %w", inboundID, err)
	}

	return nil
}

// SetClientFlow sets flow=xtls-rprx-vision on a specific inbound of a client
// via a targeted 3X-UI update (?inboundIds=<id>). Current expiryTime is passed
// through so the partial update doesn't wipe it (3X-UI replaces the client in
// the inbound settings wholesale).
func (api *PanelAPI) SetClientFlow(panel Panel, email string, inboundID int) error {
	stats, err := api.GetClientStats(panel, email)
	if err != nil {
		return fmt.Errorf("fetching client before flow update: %w", err)
	}

	_url := api.buildURL(panel, fmt.Sprintf("panel/api/clients/update/%s?inboundIds=%d", email, inboundID))

	payload := map[string]interface{}{
		"email":  email,
		"enable": true,
		"flow":   "xtls-rprx-vision",
	}
	if stats.ExpiryTime > 0 {
		payload["expiryTime"] = stats.ExpiryTime
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling flow payload: %w", err)
	}

	resp, err := api.doRequest("POST", _url, panel.Token, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("setting flow on inbound: %w", err)
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

// DetachClient detaches a client from an inbound
func (api *PanelAPI) DetachClient(panel Panel, email string, inboundID int) error {
	_url := api.buildURL(panel, fmt.Sprintf("panel/api/clients/%s/detach", email))

	payload := map[string]interface{}{
		"inboundIds": []int{inboundID},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling payload: %w", err)
	}

	resp, err := api.doRequest("POST", _url, panel.Token, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("detaching client from inbound: %w", err)
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

// UpdateClient updates client fields (expiryTime, etc.) on all attached inbounds
func (api *PanelAPI) UpdateClient(panel Panel, email string, expiryTime int64) error {
	_url := api.buildURL(panel, fmt.Sprintf("panel/api/clients/update/%s", email))

	payload := map[string]interface{}{
		"email":      email,
		"expiryTime": expiryTime,
		"enable":     true,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling payload: %w", err)
	}

	resp, err := api.doRequest("POST", _url, panel.Token, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("updating client: %w", err)
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

// GetClientKeyForInbound returns the VLESS key for a specific client on a
// specific inbound (matched by inbound port). Fails if the client/inbound
// no longer exists on the panel.
func (api *PanelAPI) GetClientKeyForInbound(panel Panel, email string, inboundID int) (VLESSKey, error) {
	inbounds, err := api.ListInbounds(panel)
	if err != nil {
		return VLESSKey{}, err
	}

	var targetPort int
	foundInbound := false
	for _, ib := range inbounds {
		if ib.ID == inboundID {
			targetPort = ib.Port
			foundInbound = true
			break
		}
	}
	if !foundInbound {
		return VLESSKey{}, fmt.Errorf("инбаунд %d не найден на панели", inboundID)
	}

	keys, err := api.GetClientKeys(panel, email)
	if err != nil {
		return VLESSKey{}, err
	}

	for _, k := range keys {
		if k.Port == targetPort {
			return k, nil
		}
	}

	return VLESSKey{}, fmt.Errorf("ключ для клиента %s на инбаунде %d не найден", email, inboundID)
}

// GetClientStats fetches traffic/expiry stats for a client from the panel's
// /panel/api/clients/list endpoint (present in 3X-UI 3.3.1+).
func (api *PanelAPI) GetClientStats(panel Panel, email string) (*Client, error) {
	clients, err := api.ListClients(panel)
	if err != nil {
		return nil, err
	}
	for i := range clients {
		if clients[i].Email == email {
			c := clients[i]
			return &c, nil
		}
	}
	return nil, fmt.Errorf("клиент %s не найден на панели", email)
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
