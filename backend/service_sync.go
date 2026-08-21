package main

import (
	"context"
	"log"
	"os"
	"os/exec"
	"time"
)

// SyncService — синхронизация файлов подписок с агрегатором (rsync-скрипт).
type SyncService struct {
	sync   *SyncState
	script string
}

func NewSyncService(sync *SyncState, script string) *SyncService {
	return &SyncService{sync: sync, script: script}
}

// Run выполняет rsync-скрипт и обновляет SyncState. При ошибке скрипта
// возвращает SyncResponse со статусом "error" (структурированное тело 502).
func (s *SyncService) Run(ctx context.Context) (SyncResponse, error) {
	if _, err := os.Stat(s.script); err != nil {
		return SyncResponse{}, errNotImplemented("sync script not found: " + s.script)
	}

	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", s.script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("SyncToAggregator failed: %v\n%s", err, string(out))
		return SyncResponse{
			Status: "error",
			Error:  err.Error(),
			Output: tailString(string(out), 2000),
		}, errBadGateway(err.Error())
	}

	// Синк прошёл: локальные файлы = агрегатор. Флаг опускаем.
	s.sync.Clear()

	return SyncResponse{Status: "synced", Output: tailString(string(out), 2000)}, nil
}
