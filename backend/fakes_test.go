package main

import (
	"errors"
	"testing"

	"vlesspanel/dto"
	"vlesspanel/model"
	"vlesspanel/xui"
)

// appErrStatus извлекает HTTP-статус из *AppError (0, если это не AppError).
func appErrStatus(t *testing.T, err error) int {
	t.Helper()
	var ae *AppError
	if errors.As(err, &ae) {
		return ae.Status
	}
	return 0
}

// fakePanelClient — фейк PanelClient (embedded-interface: не реализованные
// методы паникуют при вызове — в тестах вызываются только реализованные).
type fakePanelClient struct {
	PanelClient
	clients     []model.Client
	inbounds    []xui.XUIInbound
	keysByEmail map[string][]model.VLESSKey
	err         error
}

func (f *fakePanelClient) ListClients(model.Panel) ([]model.Client, error) {
	return f.clients, f.err
}

func (f *fakePanelClient) ListInbounds(model.Panel) ([]xui.XUIInbound, error) {
	return f.inbounds, f.err
}

func (f *fakePanelClient) ListClientsAndInbounds(model.Panel) ([]model.Client, []xui.XUIInbound, error) {
	return f.clients, f.inbounds, f.err
}

func (f *fakePanelClient) GetClientKeys(_ model.Panel, email string) ([]model.VLESSKey, error) {
	return f.keysByEmail[email], f.err
}

func (f *fakePanelClient) GetClientStats(_ model.Panel, email string) (*model.Client, error) {
	for i := range f.clients {
		if f.clients[i].Email == email {
			c := f.clients[i]
			return &c, nil
		}
	}
	return nil, ErrClientNotFound
}

func (f *fakePanelClient) GetClientKeyForInbound(_ model.Panel, email string, inboundID int) (model.VLESSKey, error) {
	if f.err != nil {
		return model.VLESSKey{}, f.err
	}
	targetPort := 0
	for _, ib := range f.inbounds {
		if ib.ID == inboundID {
			targetPort = ib.Port
			break
		}
	}
	if targetPort == 0 {
		return model.VLESSKey{}, ErrInboundNotFound
	}
	for _, k := range f.keysByEmail[email] {
		if k.Port == targetPort {
			return k, nil
		}
	}
	return model.VLESSKey{}, ErrClientNotFound
}

func (f *fakePanelClient) GetClientKeysForEmails(_ model.Panel, emails []string, _ int) (map[string][]model.VLESSKey, []xui.XUIInbound, error) {
	if f.err != nil {
		return nil, nil, f.err
	}
	out := make(map[string][]model.VLESSKey, len(emails))
	for _, e := range emails {
		out[e] = f.keysByEmail[e]
	}
	return out, f.inbounds, nil
}

func (f *fakePanelClient) CreateClient(model.Panel, int, string, string) error { return f.err }
func (f *fakePanelClient) AttachClient(model.Panel, string, int) error         { return f.err }
func (f *fakePanelClient) DetachClient(model.Panel, string, int) error         { return f.err }
func (f *fakePanelClient) UpdateClient(model.Panel, string, int64) error       { return f.err }

// fakeDaemon — фейк VlessSubTestClient.
type fakeDaemon struct {
	VlessSubTestClient
	status  dto.VlessSubTestStatus
	results []dto.TestSingleResponse
	err     error
}

func (f *fakeDaemon) Status() dto.VlessSubTestStatus { return f.status }

func (f *fakeDaemon) TestSingle(string, int) (dto.TestSingleResponse, error) {
	if f.err != nil {
		return dto.TestSingleResponse{}, f.err
	}
	if len(f.results) == 0 {
		return dto.TestSingleResponse{Status: "OK"}, nil
	}
	r := f.results[0]
	f.results = f.results[1:]
	return r, nil
}
