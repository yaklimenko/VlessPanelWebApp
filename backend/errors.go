package main

import (
	"errors"
	"fmt"
)

// Sentinel-ошибки для типовой диспетчеризации через errors.Is.
// Текст ошибок остаётся человекочитаемым (исходные сообщения сохранены),
// но добавляется тип для надёжной маршрутизации в хендлерах вместо
// strings.Contains-парсинга.

// Storage-семейство.
var (
	ErrPanelNotFound           = errors.New("panel not found")
	ErrKeySourceNotFound       = errors.New("key source not found")
	ErrSubscriptionNotFound    = errors.New("subscription not found")
	ErrInvalidSubscriptionName = errors.New("invalid subscription name")
	ErrTokenNotFound           = errors.New("token not found")
)

// PanelAPI-семейство (3X-UI).
var (
	ErrClientNotFound   = errors.New("client not found on panel")
	ErrInboundNotFound  = errors.New("inbound not found on panel")
	ErrPanelUnreachable = errors.New("panel unreachable")
)

// VlessSubTest-семейство (демон тестов).
var (
	ErrDaemonUnreachable = errors.New("daemon unreachable")
	ErrDaemonParse       = errors.New("daemon parse error")
)

// Sync-семейство (агрегатор).
var (
	ErrSyncScriptNotFound = errors.New("sync script not found")
)

// AppError — ошибка уровня use-case, несущая HTTP-статус и user-facing
// сообщение. Сервисы возвращают *AppError для доменных/валидационных ошибок,
// не завися от net/http в самих сервисах (статус — просто int).
type AppError struct {
	Status  int
	Message string
}

func (e *AppError) Error() string { return e.Message }

func appErr(status int, format string, a ...interface{}) *AppError {
	return &AppError{Status: status, Message: fmt.Sprintf(format, a...)}
}

func errNotFound(format string, a ...interface{}) *AppError {
	return appErr(404, format, a...)
}

func errBadRequest(format string, a ...interface{}) *AppError {
	return appErr(400, format, a...)
}

func errConflict(format string, a ...interface{}) *AppError {
	return appErr(409, format, a...)
}

func errBadGateway(format string, a ...interface{}) *AppError {
	return appErr(502, format, a...)
}

func errNotImplemented(format string, a ...interface{}) *AppError {
	return appErr(501, format, a...)
}

func errInternal(format string, a ...interface{}) *AppError {
	return appErr(500, format, a...)
}
