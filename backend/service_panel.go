package main

import (
	"log"
	"time"

	"vlesspanel/dto"
	"vlesspanel/model"
)

// PanelService — use cases для панелей и клиентов/инбаундов 3X-UI.
type PanelService struct {
	storage  Repository
	panelAPI PanelClient
}

func NewPanelService(storage Repository, panelAPI PanelClient) *PanelService {
	return &PanelService{storage: storage, panelAPI: panelAPI}
}

func (s *PanelService) List() ([]model.Panel, error) {
	panels, err := s.storage.LoadPanels()
	if err != nil {
		return nil, errInternal(msgLoadPanelsFailed)
	}
	return panels, nil
}

func (s *PanelService) Create(req dto.CreatePanelRequest) (model.Panel, error) {
	if req.Name == "" || req.URL == "" || req.Token == "" {
		return model.Panel{}, errBadRequest(msgPanelFieldsRequired)
	}
	panel, err := s.storage.AddPanel(req)
	if err != nil {
		return model.Panel{}, errInternal(msgCreatePanelFailed)
	}
	return panel, nil
}

func (s *PanelService) Delete(id string) error {
	return s.storage.DeletePanel(id)
}

func (s *PanelService) ListClients(panelID string) ([]model.Client, error) {
	panel, err := s.storage.GetPanel(panelID)
	if err != nil {
		return nil, err
	}
	clients, err := s.panelAPI.ListClients(panel)
	if err != nil {
		log.Printf("list clients: %v", err)
		return nil, errInternal(msgListClientsFailed)
	}
	return clients, nil
}

func (s *PanelService) CreateClient(panelID string, req dto.CreateClientRequest) (dto.StatusResponse, error) {
	if req.Email == "" || req.InboundID == 0 {
		return dto.StatusResponse{}, errBadRequest(msgClientFieldsRequired)
	}
	panel, err := s.storage.GetPanel(panelID)
	if err != nil {
		return dto.StatusResponse{}, err
	}
	if err := s.panelAPI.CreateClient(panel, req.InboundID, req.Email, req.ExpiryDate); err != nil {
		log.Printf("create client: %v", err)
		return dto.StatusResponse{}, errInternal(msgCreateClientFailed)
	}
	return dto.StatusResponse{Status: "created", Email: req.Email}, nil
}

func (s *PanelService) GetClientKeys(panelID, email string) ([]model.VLESSKey, error) {
	panel, err := s.storage.GetPanel(panelID)
	if err != nil {
		return nil, err
	}
	keys, err := s.panelAPI.GetClientKeys(panel, email)
	if err != nil {
		log.Printf("get client keys: %v", err)
		return nil, errInternal(msgGetKeysFailed)
	}
	return keys, nil
}

func (s *PanelService) ListInbounds(panelID string) ([]dto.SimpleInbound, error) {
	panel, err := s.storage.GetPanel(panelID)
	if err != nil {
		return nil, err
	}
	inbounds, err := s.panelAPI.ListInbounds(panel)
	if err != nil {
		log.Printf("list inbounds: %v", err)
		return nil, errInternal(msgListInboundsFailed)
	}
	simple := make([]dto.SimpleInbound, 0, len(inbounds))
	for _, ib := range inbounds {
		simple = append(simple, dto.SimpleInbound{
			ID:       ib.ID,
			Remark:   ib.Remark,
			Port:     ib.Port,
			Protocol: ib.Protocol,
			Enable:   ib.Enable,
		})
	}
	return simple, nil
}

func (s *PanelService) Attach(panelID, email string, inboundID int) (dto.StatusResponse, error) {
	if inboundID == 0 {
		return dto.StatusResponse{}, errBadRequest(msgInboundRequired)
	}
	panel, err := s.storage.GetPanel(panelID)
	if err != nil {
		return dto.StatusResponse{}, err
	}
	if err := s.panelAPI.AttachClient(panel, email, inboundID); err != nil {
		log.Printf("attach inbound: %v", err)
		return dto.StatusResponse{}, errInternal(msgAttachFailed)
	}
	return dto.StatusResponse{Status: "attached", Email: email}, nil
}

func (s *PanelService) Detach(panelID, email string, inboundID int) (dto.StatusResponse, error) {
	if inboundID == 0 {
		return dto.StatusResponse{}, errBadRequest(msgInboundRequired)
	}
	panel, err := s.storage.GetPanel(panelID)
	if err != nil {
		return dto.StatusResponse{}, err
	}
	if err := s.panelAPI.DetachClient(panel, email, inboundID); err != nil {
		log.Printf("detach inbound: %v", err)
		return dto.StatusResponse{}, errInternal(msgDetachFailed)
	}
	return dto.StatusResponse{Status: "detached", Email: email}, nil
}

func (s *PanelService) UpdateClient(panelID, email string, req dto.UpdateClientRequest) (dto.StatusResponse, error) {
	panel, err := s.storage.GetPanel(panelID)
	if err != nil {
		return dto.StatusResponse{}, err
	}

	var expiryTime int64
	if req.ExpiryDate != "" {
		t, err := time.Parse("2006-01-02", req.ExpiryDate)
		if err != nil {
			return dto.StatusResponse{}, errBadRequest(msgBadExpiryDate)
		}
		expiryTime = t.Unix() * 1000
	}

	if err := s.panelAPI.UpdateClient(panel, email, expiryTime); err != nil {
		log.Printf("update client: %v", err)
		return dto.StatusResponse{}, errInternal(msgUpdateClientFailed)
	}
	return dto.StatusResponse{Status: "updated", Email: email}, nil
}
