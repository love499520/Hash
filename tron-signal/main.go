package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

/* ================== 数据结构 ================== */

type APIKeyConfig struct {
	Keys []string `json:"keys"`
}

type RuleConfig struct {
	TriggerEnabled   bool   `json:"trigger_enabled"`
	TriggerState     string `json:"trigger_state"`
	TriggerThreshold int    `json:"trigger_threshold"`

	HitEnabled bool   `json:"hit_enabled"`
	HitExpect  string `json:"hit_expect"`
	HitOffset  int    `json:"hit_offset"`
}

type Signal struct {
	Signal      string `json:"signal"`
	BaseSignal  string `json:"base_signal,omitempty"`
	Height      int64  `json:"height"`
	BaseHeight  int64  `json:"base_height,omitempty"`
	Time        string `json:"time"`
}

/* ================== 全局状态 ================== */

var (
	apiKeys APIKeyConfig
	rules   RuleConfig

	lastBlockHeight int64
	counter         int
	waitReverse     bool = true

	wsClients   = make(map[*websocket.Conn]bool)
	wsClientsMu sync.Mutex
)

/* ==
