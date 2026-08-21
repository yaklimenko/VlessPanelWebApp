package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"time"
)

// ksLabelFor builds a human-readable label for a KeySource.
func ksLabelFor(ks *KeySource, panelMap map[string]Panel) string {
	if ks.Label != "" {
		return ks.Label
	}
	if ks.Type == "manual" {
		return "manual"
	}
	p := panelMap[ks.PanelID]
	return fmt.Sprintf("%s · %s", p.Name, ks.ClientEmail)
}

func keySourceInKeys(keys []SubKey, ksID string) bool {
	for _, k := range keys {
		if k.KeySourceID != nil && *k.KeySourceID == ksID {
			return true
		}
	}
	return false
}

func countKind(items []GenerationReportItem, kind string) int {
	n := 0
	for _, it := range items {
		if it.Kind == kind {
			n++
		}
	}
	return n
}

func strPtr(s string) *string { return &s }

// nowStr returns the current time as RFC3339 UTC.
func nowStr() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// randID returns a short random hex id.
func randID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// newRawToken генерирует новый API-токен: "vlt_" + 48 hex-символов (192 бит).
func newRawToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "vlt_" + fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return "vlt_" + hex.EncodeToString(b)
}

// validSubscriptionName restricts subscription names to safe file characters.
func validSubscriptionName(name string) bool {
	if name == "" || len(name) > 60 {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

// labelFromVless derives a label from the fragment of a vless link.
func labelFromVless(link string) string {
	if u, err := url.Parse(link); err == nil && u.Fragment != "" {
		return u.Fragment
	}
	return "manual"
}

func tailString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
