package main

import "sync/atomic"

// SyncState отслеживает, нужны ли изменения на агрегаторе после локальных мутаций.
// Флаг поднимается Mark() только тогда, когда изменился файл подписки
// (configs-{name}.txt) — других причин нет. Сбрасывается Clear() сразу после
// успешного синка с агрегатором.
//
// Для одного булева флага sync.Mutex избыточен: конкурентные чтение/запись
// (enrichSync / Mark / Clear) происходят из разных goroutine net/http, поэтому
// используется atomic.Bool — ноль локов, race-safe.
type SyncState struct {
	needed atomic.Bool
}

// NewSyncState создаёт SyncState в состоянии «требуется синк» — при старте
// статус агрегатора неизвестен, поэтому честно показываем «нужна сверка».
func NewSyncState() *SyncState {
	s := &SyncState{}
	s.needed.Store(true)
	return s
}

// Mark поднимает флаг «нужна сверка с агрегатором» после изменения файла подписки.
func (s *SyncState) Mark() { s.needed.Store(true) }

// Clear опускает флаг после успешного синка с агрегатором.
func (s *SyncState) Clear() { s.needed.Store(false) }

// Needed сообщает, требуется ли синк с агрегатором.
func (s *SyncState) Needed() bool { return s.needed.Load() }
