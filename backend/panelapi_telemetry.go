package main

import (
	"encoding/json"
	"fmt"
	"strconv"

	"vlesspanel/model"
	"vlesspanel/xui"
)

// Телеметрия панелей 3X-UI (раздел статистики, Этап 1).
// Эндпоинты по OpenAPI 3.7.0 (spec/3xui/3.7.0/openapi.json):
//   - GET /panel/api/server/status — живой снапшот (CPU/RAM/swap/disk/netIO/load/tcpCount/xray)
//   - GET /panel/api/server/history/{metric}/{bucket} — история метрики за ~6 часов.
//     ⚠️ Реально работают только bucket 2/30/60 (120/180/300 → invalid bucket).
//
// Оба эндпоинта требуют Authorization: Bearer <token> (токен из panels.json) —
// это уже делает doRequest.

// ServerStatus возвращает живой снапшот системы панели.
func (api *PanelAPI) ServerStatus(panel model.Panel) (*xui.ServerStatus, error) {
	url := api.buildURL(panel, "panel/api/server/status")

	resp, err := api.doRequest("GET", url, panel, nil)
	if err != nil {
		return nil, fmt.Errorf("server status: %w", err)
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
		return nil, fmt.Errorf("marshaling status obj: %w", err)
	}

	var st xui.ServerStatus
	if err := json.Unmarshal(objBytes, &st); err != nil {
		return nil, fmt.Errorf("parsing server status: %w", err)
	}
	return &st, nil
}

// ServerHistory возвращает историю метрики с панели.
// bucket — размер бакета в секундах (работают 2/30/60).
// Точки отсортированы по времени; t — unix-секунды.
func (api *PanelAPI) ServerHistory(panel model.Panel, metric string, bucket int) ([]xui.HistoryPoint, error) {
	url := api.buildURL(panel, "panel/api/server/history/"+metric+"/"+strconv.Itoa(bucket))

	resp, err := api.doRequest("GET", url, panel, nil)
	if err != nil {
		return nil, fmt.Errorf("server history %s: %w", metric, err)
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
		return nil, fmt.Errorf("marshaling history obj: %w", err)
	}

	var points []xui.HistoryPoint
	if err := json.Unmarshal(objBytes, &points); err != nil {
		return nil, fmt.Errorf("parsing history %s: %w", metric, err)
	}
	return points, nil
}
