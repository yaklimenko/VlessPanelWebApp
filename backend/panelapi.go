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
	"sync"
	"time"

	"vlesspanel/model"
	"vlesspanel/xui"
)

// PanelAPI handles communication with 3X-UI panels
type PanelAPI struct {
	secure   *http.Client // верификация TLS ВКЛ (дефолт)
	insecure *http.Client // InsecureSkipVerify для self-signed панелей
}

// NewPanelAPI creates a new PanelAPI.
// The timeout is 10 seconds per operation with 3X-UI (per spec).
func NewPanelAPI() *PanelAPI {
	secure := &http.Client{Timeout: 10 * time.Second}
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	insecure := &http.Client{
		Transport: tr,
		Timeout:   10 * time.Second,
	}
	return &PanelAPI{
		secure:   secure,
		insecure: insecure,
	}
}

// clientFor возвращает клиент в зависимости от per-panel флага TLS-верификации.
func (api *PanelAPI) clientFor(panel model.Panel) *http.Client {
	if panel.InsecureSkipVerify {
		return api.insecure
	}
	return api.secure
}

// buildURL constructs the full API URL for a panel endpoint
func (api *PanelAPI) buildURL(panel model.Panel, path string) string {
	base := strings.TrimRight(panel.URL, "/")
	if panel.WebBasePath != "" {
		base = strings.TrimRight(base+"/"+strings.Trim(panel.WebBasePath, "/"), "/")
	}
	return fmt.Sprintf("%s/%s", base, strings.TrimLeft(path, "/"))
}

// doRequest performs an HTTP request to a 3X-UI panel.
// The client is chosen per-panel (TLS verification on/off). Network-level
// failures (timeout, connection refused, DNS, TLS, EOF) are wrapped as
// ErrPanelUnreachable so callers can dispatch via errors.Is instead of
// strings.Contains-парсинга.
func (api *PanelAPI) doRequest(method, url string, panel model.Panel, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+panel.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := api.clientFor(panel).Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPanelUnreachable, err)
	}
	return resp, nil
}

// parseResponse parses a 3X-UI API response
func (api *PanelAPI) parseResponse(resp *http.Response) (*xui.XUIResponse, error) {
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("3X-UI HTTP %d: %s", resp.StatusCode, string(data))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data))
	}

	var xuiResp xui.XUIResponse
	if err := json.Unmarshal(data, &xuiResp); err != nil {
		log.Printf("parseResponse: failed to unmarshal 3X-UI response: %v\nBody: %s", err, string(data))
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	return &xuiResp, nil
}

// ListInbounds fetches all inbounds from a panel
func (api *PanelAPI) ListInbounds(panel model.Panel) ([]xui.XUIInbound, error) {
	url := api.buildURL(panel, "panel/api/inbounds/list")

	resp, err := api.doRequest("GET", url, panel, nil)
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

	// Parse the obj field into []xui.XUIInbound
	objBytes, err := json.Marshal(xuiResp.Obj)
	if err != nil {
		return nil, fmt.Errorf("marshaling obj: %w", err)
	}

	var inbounds []xui.XUIInbound
	if err := json.Unmarshal(objBytes, &inbounds); err != nil {
		log.Printf("ListInbounds: failed to unmarshal inbounds array: %v\nObj JSON: %s", err, string(objBytes))
		return nil, fmt.Errorf("parsing inbounds: %w", err)
	}

	return inbounds, nil
}

// fetchXUIClients fetches raw clients from /panel/api/clients/list (3X-UI v3.4.2+)
func (api *PanelAPI) fetchXUIClients(panel model.Panel) ([]xui.XUIClient, error) {
	url := api.buildURL(panel, "panel/api/clients/list")

	resp, err := api.doRequest("GET", url, panel, nil)
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

	var xuiClients []xui.XUIClient
	if err := json.Unmarshal(objBytes, &xuiClients); err != nil {
		log.Printf("fetchXUIClients: failed to unmarshal clients: %v\nObj: %s", err, string(objBytes))
		return nil, fmt.Errorf("parsing clients: %w", err)
	}

	return xuiClients, nil
}

// mapClientsToInbounds converts raw XUI clients into model.Client, resolving inbound
// IDs to remark names via the inbounds list.
func mapClientsToInbounds(xuiClients []xui.XUIClient, inbounds []xui.XUIInbound) []model.Client {
	remarkMap := make(map[int]string, len(inbounds))
	for _, ib := range inbounds {
		remarkMap[ib.ID] = ib.Remark
	}

	clients := make([]model.Client, 0, len(xuiClients))
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
		clients = append(clients, model.Client{
			ID:         xc.Email,
			Email:      xc.Email,
			Enable:     xc.Enable,
			ExpiryTime: xc.ExpiryTime,
			InboundIDs: xc.InboundIDs,
			Inbounds:   inboundRemarks,
			Keys:       []model.VLESSKey{},
			Up:         xc.Traffic.Up,
			Down:       xc.Traffic.Down,
		})
	}

	return clients
}

// ListClientsDirect uses the dedicated /panel/api/clients/list endpoint (3X-UI v3.4.2+)
func (api *PanelAPI) ListClientsDirect(panel model.Panel) ([]model.Client, error) {
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
func (api *PanelAPI) ListClientsAndInbounds(panel model.Panel) ([]model.Client, []xui.XUIInbound, error) {
	type res struct {
		kind     string // "clients" | "inbounds"
		clients  []xui.XUIClient
		inbounds []xui.XUIInbound
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

	var xuiClients []xui.XUIClient
	var inbounds []xui.XUIInbound
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
func (api *PanelAPI) ListClients(panel model.Panel) ([]model.Client, error) {
	clients, err := api.ListClientsDirect(panel)
	if err == nil {
		return clients, nil
	}
	log.Printf("ListClientsDirect failed, falling back to inbounds-based method: %v", err)
	return api.listClientsFromInbounds(panel)
}

// listClientsFromInbounds extracts all clients from all inbounds (legacy method)
func (api *PanelAPI) listClientsFromInbounds(panel model.Panel) ([]model.Client, error) {
	inbounds, err := api.ListInbounds(panel)
	if err != nil {
		return nil, err
	}

	return clientsFromInbounds(inbounds), nil
}

// clientsFromInbounds extracts all clients from an already-fetched inbounds list.
func clientsFromInbounds(inbounds []xui.XUIInbound) []model.Client {
	clientMap := make(map[string]*model.Client)

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
				client := model.Client{
					ID:         clientID,
					Email:      stat.Email,
					Enable:     stat.Enable,
					InboundIDs: []int{inbound.ID},
					Inbounds:   []string{inbound.Remark},
					Keys:       []model.VLESSKey{},
					Up:         stat.Up,
					Down:       stat.Down,
				}
				clientMap[clientID] = &client
			}
		}
	}

	clients := make([]model.Client, 0, len(clientMap))
	for _, c := range clientMap {
		clients = append(clients, *c)
	}

	return clients
}

// GetClientLinks fetches the links the panel itself generates for a client
// (the same strings the panel UI's Copy URL button copies), keyed by inbound port.
func (api *PanelAPI) GetClientLinks(panel model.Panel, email string) (map[int]string, error) {
	_url := api.buildURL(panel, "panel/api/clients/links/"+url.PathEscape(email))

	resp, err := api.doRequest("GET", _url, panel, nil)
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
func (api *PanelAPI) GetClientKeys(panel model.Panel, email string) ([]model.VLESSKey, error) {
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

	return buildClientKeys(panel, inbounds, panelLinks, email), nil
}

// GetClientKeysForEmails resolves VLESS keys for multiple client emails on one
// panel using a single inbounds snapshot. Link fetches per email run with
// bounded concurrency. Returns keys grouped by email and the inbounds snapshot
// (caller maps inboundID → port). Used by regenerate-all to avoid the N+1
// pattern of GetClientKeyForInbound.
func (api *PanelAPI) GetClientKeysForEmails(panel model.Panel, emails []string, concurrency int) (map[string][]model.VLESSKey, []xui.XUIInbound, error) {
	inbounds, err := api.ListInbounds(panel)
	if err != nil {
		return nil, nil, err
	}

	if concurrency < 1 {
		concurrency = 1
	}
	keys := make(map[string][]model.VLESSKey, len(emails))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)

	for _, email := range emails {
		wg.Add(1)
		go func(email string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			panelLinks, linkErr := api.GetClientLinks(panel, email)
			if linkErr != nil {
				log.Printf("GetClientKeysForEmails: panel links unavailable for %s: %v", email, linkErr)
			}
			ks := buildClientKeys(panel, inbounds, panelLinks, email)
			mu.Lock()
			keys[email] = ks
			mu.Unlock()
		}(email)
	}
	wg.Wait()

	return keys, inbounds, nil
}

// buildClientKeys constructs VLESS keys for one client across all inbounds from
// an already-fetched inbounds snapshot and the panel's links (keyed by port).
// Pure: no HTTP calls.
func buildClientKeys(panel model.Panel, inbounds []xui.XUIInbound, panelLinks map[int]string, email string) []model.VLESSKey {
	var keys []model.VLESSKey

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
			log.Printf("buildClientKeys: failed to unmarshal settings for inbound %d (%s) while getting keys for %s: %v\nSettings: %s",
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

			keys = append(keys, model.VLESSKey{
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

	return keys
}

// CreateClient creates a new client on a panel's inbounds (3X-UI v3.5.0+)
func (api *PanelAPI) CreateClient(panel model.Panel, inboundID int, email string, expiryDate string) error {
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

	resp, err := api.doRequest("POST", url, panel, bytes.NewReader(body))
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
func (api *PanelAPI) AttachClient(panel model.Panel, email string, inboundID int) error {
	_url := api.buildURL(panel, fmt.Sprintf("panel/api/clients/%s/attach", email))

	payload := map[string]interface{}{
		"inboundIds": []int{inboundID},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling payload: %w", err)
	}

	resp, err := api.doRequest("POST", _url, panel, bytes.NewReader(body))
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
func (api *PanelAPI) SetClientFlow(panel model.Panel, email string, inboundID int) error {
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

	resp, err := api.doRequest("POST", _url, panel, bytes.NewReader(body))
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
func (api *PanelAPI) DetachClient(panel model.Panel, email string, inboundID int) error {
	_url := api.buildURL(panel, fmt.Sprintf("panel/api/clients/%s/detach", email))

	payload := map[string]interface{}{
		"inboundIds": []int{inboundID},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling payload: %w", err)
	}

	resp, err := api.doRequest("POST", _url, panel, bytes.NewReader(body))
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

// UpdateClient updates client fields (expiryTime, etc.) on all attached inbounds.
// flow=xtls-rprx-vision is always sent: 3X-UI's Update replaces the client in
// each inbound's settings, so omitting flow would silently strip it (an empty
// flow survives clientWithInboundFlow on flow-eligible inbounds).
func (api *PanelAPI) UpdateClient(panel model.Panel, email string, expiryTime int64) error {
	_url := api.buildURL(panel, fmt.Sprintf("panel/api/clients/update/%s", email))

	payload := map[string]interface{}{
		"email":  email,
		"enable": true,
		"flow":   "xtls-rprx-vision",
	}
	if expiryTime > 0 {
		payload["expiryTime"] = expiryTime
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling payload: %w", err)
	}

	resp, err := api.doRequest("POST", _url, panel, bytes.NewReader(body))
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
func (api *PanelAPI) GetClientKeyForInbound(panel model.Panel, email string, inboundID int) (model.VLESSKey, error) {
	inbounds, err := api.ListInbounds(panel)
	if err != nil {
		return model.VLESSKey{}, err
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
		return model.VLESSKey{}, fmt.Errorf("%w: inbound %d", ErrInboundNotFound, inboundID)
	}

	keys, err := api.GetClientKeys(panel, email)
	if err != nil {
		return model.VLESSKey{}, err
	}

	for _, k := range keys {
		if k.Port == targetPort {
			return k, nil
		}
	}

	return model.VLESSKey{}, fmt.Errorf("%w: client %s on inbound %d", ErrClientNotFound, email, inboundID)
}

// GetClientStats fetches traffic/expiry stats for a client from the panel's
// /panel/api/clients/list endpoint (present in 3X-UI 3.3.1+).
func (api *PanelAPI) GetClientStats(panel model.Panel, email string) (*model.Client, error) {
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
	return nil, fmt.Errorf("%w: %s", ErrClientNotFound, email)
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
