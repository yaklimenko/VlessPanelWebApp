package main

import "time"

// PanelService — use cases для панелей и клиентов/инбаундов 3X-UI.
type PanelService struct {
	storage  *Storage
	panelAPI *PanelAPI
}

func NewPanelService(storage *Storage, panelAPI *PanelAPI) *PanelService {
	return &PanelService{storage: storage, panelAPI: panelAPI}
}

func (s *PanelService) List() ([]Panel, error) {
	panels, err := s.storage.LoadPanels()
	if err != nil {
		return nil, errInternal("Failed to load panels")
	}
	return panels, nil
}

func (s *PanelService) Create(req CreatePanelRequest) (Panel, error) {
	if req.Name == "" || req.URL == "" || req.Token == "" {
		return Panel{}, errBadRequest("name, url, and token are required")
	}
	panel, err := s.storage.AddPanel(req)
	if err != nil {
		return Panel{}, errInternal("Failed to create panel")
	}
	return panel, nil
}

func (s *PanelService) Delete(id string) error {
	return s.storage.DeletePanel(id)
}

func (s *PanelService) ListClients(panelID string) ([]Client, error) {
	panel, err := s.storage.GetPanel(panelID)
	if err != nil {
		return nil, err
	}
	clients, err := s.panelAPI.ListClients(panel)
	if err != nil {
		return nil, errInternal("Failed to list clients: %v", err)
	}
	return clients, nil
}

func (s *PanelService) CreateClient(panelID string, req CreateClientRequest) (StatusResponse, error) {
	if req.Email == "" || req.InboundID == 0 {
		return StatusResponse{}, errBadRequest("email and inboundId are required")
	}
	panel, err := s.storage.GetPanel(panelID)
	if err != nil {
		return StatusResponse{}, err
	}
	if err := s.panelAPI.CreateClient(panel, req.InboundID, req.Email, req.ExpiryDate); err != nil {
		return StatusResponse{}, errInternal("Failed to create client: %v", err)
	}
	return StatusResponse{Status: "created", Email: req.Email}, nil
}

func (s *PanelService) GetClientKeys(panelID, email string) ([]VLESSKey, error) {
	panel, err := s.storage.GetPanel(panelID)
	if err != nil {
		return nil, err
	}
	keys, err := s.panelAPI.GetClientKeys(panel, email)
	if err != nil {
		return nil, errInternal("Failed to get keys: %v", err)
	}
	return keys, nil
}

func (s *PanelService) ListInbounds(panelID string) ([]SimpleInbound, error) {
	panel, err := s.storage.GetPanel(panelID)
	if err != nil {
		return nil, err
	}
	inbounds, err := s.panelAPI.ListInbounds(panel)
	if err != nil {
		return nil, errInternal("Failed to list inbounds: %v", err)
	}
	simple := make([]SimpleInbound, 0, len(inbounds))
	for _, ib := range inbounds {
		simple = append(simple, SimpleInbound{
			ID:       ib.ID,
			Remark:   ib.Remark,
			Port:     ib.Port,
			Protocol: ib.Protocol,
			Enable:   ib.Enable,
		})
	}
	return simple, nil
}

func (s *PanelService) Attach(panelID, email string, inboundID int) (StatusResponse, error) {
	if inboundID == 0 {
		return StatusResponse{}, errBadRequest("inboundId is required")
	}
	panel, err := s.storage.GetPanel(panelID)
	if err != nil {
		return StatusResponse{}, err
	}
	if err := s.panelAPI.AttachClient(panel, email, inboundID); err != nil {
		return StatusResponse{}, errInternal("Failed to attach inbound: %v", err)
	}
	return StatusResponse{Status: "attached", Email: email}, nil
}

func (s *PanelService) Detach(panelID, email string, inboundID int) (StatusResponse, error) {
	if inboundID == 0 {
		return StatusResponse{}, errBadRequest("inboundId is required")
	}
	panel, err := s.storage.GetPanel(panelID)
	if err != nil {
		return StatusResponse{}, err
	}
	if err := s.panelAPI.DetachClient(panel, email, inboundID); err != nil {
		return StatusResponse{}, errInternal("Failed to detach inbound: %v", err)
	}
	return StatusResponse{Status: "detached", Email: email}, nil
}

func (s *PanelService) UpdateClient(panelID, email string, req UpdateClientRequest) (StatusResponse, error) {
	panel, err := s.storage.GetPanel(panelID)
	if err != nil {
		return StatusResponse{}, err
	}

	var expiryTime int64
	if req.ExpiryDate != "" {
		t, err := time.Parse("2006-01-02", req.ExpiryDate)
		if err != nil {
			return StatusResponse{}, errBadRequest("Invalid expiryDate format (expected YYYY-MM-DD)")
		}
		expiryTime = t.Unix() * 1000
	}

	if err := s.panelAPI.UpdateClient(panel, email, expiryTime); err != nil {
		return StatusResponse{}, errInternal("Failed to update client: %v", err)
	}
	return StatusResponse{Status: "updated", Email: email}, nil
}
