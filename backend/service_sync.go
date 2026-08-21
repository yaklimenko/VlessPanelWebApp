package main

import (
	"context"
	"errors"
	"log"
	"time"

	"vlesspanel/dto"
)

// SyncService — синхронизация файлов подписок с агрегатором (через AggregatorSyncer).
type SyncService struct {
	sync   *SyncState
	syncer AggregatorSyncer
}

func NewSyncService(sync *SyncState, syncer AggregatorSyncer) *SyncService {
	return &SyncService{sync: sync, syncer: syncer}
}

// Run выполняет синк и обновляет SyncState. При ошибке скрипта возвращает
// dto.SyncResponse со статусом "error" (структурированное тело 502).
func (s *SyncService) Run(ctx context.Context) (dto.SyncResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	out, err := s.syncer.Sync(ctx)
	if err != nil {
		if errors.Is(err, ErrSyncScriptNotFound) {
			return dto.SyncResponse{}, errNotImplemented(err.Error())
		}
		log.Printf("SyncToAggregator failed: %v\n%s", err, out)
		return dto.SyncResponse{
			Status: "error",
			Error:  err.Error(),
			Output: tailString(out, 2000),
		}, errBadGateway(err.Error())
	}

	// Синк прошёл: локальные файлы = агрегатор. Флаг опускаем.
	s.sync.Clear()

	return dto.SyncResponse{Status: "synced", Output: tailString(out, 2000)}, nil
}
