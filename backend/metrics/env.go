package metrics

import (
	"log"
	"os"
	"strconv"
	"time"
)

// envDuration/envFloat/envInt/envBool — копии одноимённых хелперов из config.go
// корневого backend: подпакет metrics standalone (не импортирует корневой
// пакет), а конфигурация алертов читается из env. Поведение идентично.
func envDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		log.Printf("config: invalid duration for %s (%q), using %s", key, v, def)
		return def
	}
	return d
}

// envFloat читает float64 из env-переменной с дефолтом def.
func envFloat(key string, def float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		log.Printf("config: invalid float for %s (%q), using %v", key, v, def)
		return def
	}
	return f
}

// envInt читает int из env-переменной с дефолтом def.
func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		log.Printf("config: invalid int for %s (%q), using %d", key, v, def)
		return def
	}
	return n
}

// envBool читает bool из env-переменной с дефолтом def.
func envBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		log.Printf("config: invalid bool for %s (%q), using %v", key, v, def)
		return def
	}
	return b
}
