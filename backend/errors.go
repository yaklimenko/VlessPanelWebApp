package main

import "errors"

// Sentinel-ошибки для типовой диспетчеризации через errors.Is.
// Текст ошибок остаётся человекочитаемым (исходные сообщения сохранены),
// но добавляется тип для надёжной маршрутизации в хендлерах вместо
// strings.Contains-парсинга.

// Storage-семейство.
var (
	ErrPanelNotFound        = errors.New("panel not found")
	ErrKeySourceNotFound    = errors.New("key source not found")
	ErrSubscriptionNotFound = errors.New("subscription not found")
)

// PanelAPI-семейство (3X-UI).
var (
	ErrClientNotFound   = errors.New("client not found on panel")
	ErrInboundNotFound  = errors.New("inbound not found on panel")
	ErrPanelUnreachable = errors.New("panel unreachable")
)
