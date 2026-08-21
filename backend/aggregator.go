package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// scriptSyncer — запускает shell-скрипт синхронизации с агрегатором.
type scriptSyncer struct {
	script string
}

func NewScriptSyncer(script string) *scriptSyncer {
	return &scriptSyncer{script: script}
}

// Sync выполняет скрипт и возвращает его вывод. Отсутствие скрипта —
// ErrSyncScriptNotFound; ошибка выполнения — оригинальная ошибка.
func (s *scriptSyncer) Sync(ctx context.Context) (string, error) {
	if _, err := os.Stat(s.script); err != nil {
		return "", fmt.Errorf("%w: %s", ErrSyncScriptNotFound, s.script)
	}
	cmd := exec.CommandContext(ctx, "bash", s.script)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
