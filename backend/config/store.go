package config

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"tron-signal/backend/judge"
	"tron-signal/backend/machine"
	"tron-signal/backend/source"
)

type Store struct {
	mu   sync.RWMutex
	path string
	data StoreData
}

func MustLoad(path string) *Store {
	s := &Store{path: path}
	_ = s.loadOrInit()
	return s
}

func (s *Store) loadOrInit() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_ = os.MkdirAll(filepath.Dir(s.path), 0755)

	b, err := os.ReadFile(s.path)
	if err != nil {
		// init defaults
		s.data = StoreData{
			Version:  1,
			Tokens:   []string{},
			Whitelist: []string{},
			JudgeRule: judge.Lucky,
			Machines:  []machine.Config{},
			Sources:   []source.Config{},
			Poll: PollConfig{
				BaseTickMS:  500,
				Auto:        true,
				WaitMinutes: 1,
			},
		}
		return s.saveLocked()
	}
	var d StoreData
	if err := json.Unmarshal(b, &d); err != nil {
		// 读坏了也不 panic：用最小默认值
		d = StoreData{Version: 1, JudgeRule: judge.Lucky}
	}
	if d.Poll.BaseTickMS <= 0 {
		d.Poll.BaseTickMS = 500
	}
	if d.Poll.WaitMinutes <= 0 {
		d.Poll.WaitMinutes = 1
	}
	if d.JudgeRule == "" {
		d.JudgeRule = judge.Lucky
	}
	s.data = d
	return nil
}

func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	b, _ := json.MarshalIndent(s.data, "", "  ")
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// ===== getters used by main.go =====

func (s *Store) GetJudgeRule() judge.RuleType {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.JudgeRule
}

func (s *Store) GetMachines() []machine.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]machine.Config, 0, len(s.data.Machines))
	out = append(out, s.data.Machines...)
	return out
}

func (s *Store) GetSources() []source.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]source.Config, 0, len(s.data.Sources))
	out = append(out, s.data.Sources...)
	return out
}

func (s *Store) GetPollConfig() PollConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.Poll
}

// ===== implements httpapi.ConfigReader interface =====

func (s *Store) GetWhitelist() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.data.Whitelist))
	out = append(out, s.data.Whitelist...)
	return out
}

func (s *Store) HasToken(token string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.data.Tokens {
		if t == token {
			return true
		}
	}
	return false
}

func (s *Store) CheckAdmin(username, password string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.data.Admin.Username == "" {
		return false
	}
	if username != s.data.Admin.Username {
		return false
	}
	salt, err := hex.DecodeString(s.data.Admin.SaltHex)
	if err != nil || len(salt) == 0 {
		return false
	}
	sum := sha256.Sum256(append(salt, []byte(password)...))
	return hex.EncodeToString(sum[:]) == s.data.Admin.HashHex
}

func (s *Store) SetAdmin(username, password string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	sum := sha256.Sum256(append(salt, []byte(password)...))

	s.data.Admin.Username = username
	s.data.Admin.SaltHex = hex.EncodeToString(salt)
	s.data.Admin.HashHex = hex.EncodeToString(sum[:])
	_ = s.saveLocked()
}

func (s *Store) AddToken(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.data.Tokens {
		if t == token {
			return
		}
	}
	s.data.Tokens = append(s.data.Tokens, token)
	_ = s.saveLocked()
}

func (s *Store) DeleteToken(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.data.Tokens))
	for _, t := range s.data.Tokens {
		if t != token {
			out = append(out, t)
		}
	}
	s.data.Tokens = out
	_ = s.saveLocked()
}

func (s *Store) SetWhitelist(list []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Whitelist = list
	_ = s.saveLocked()
}

func (s *Store) SetJudgeRule(rule judge.RuleType) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.JudgeRule = rule
	_ = s.saveLocked()
}

func (s *Store) UpsertSource(sc source.Config) {
	s.mu.Lock()
	defer s.mu.Unlock()

	found := false
	for i := range s.data.Sources {
		if s.data.Sources[i].ID == sc.ID {
			s.data.Sources[i] = sc
			found = true
			break
		}
	}
	if !found {
		s.data.Sources = append(s.data.Sources, sc)
	}
	_ = s.saveLocked()
}

func (s *Store) DeleteSource(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]source.Config, 0, len(s.data.Sources))
	for _, x := range s.data.Sources {
		if x.ID != id {
			out = append(out, x)
		}
	}
	s.data.Sources = out
	_ = s.saveLocked()
}

func (s *Store) SetSourceExtra(id string, ex any) {
	// 预留：如果你想把某些 UI 辅助字段写回 config
	_ = id
	_ = ex
	_ = s.Save()
}

func (s *Store) SetMachines(ms []machine.Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Machines = ms
	_ = s.saveLocked()
}

func (s *Store) SetPoll(baseTickMS int, auto bool, waitMinutes int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if baseTickMS > 0 {
		s.data.Poll.BaseTickMS = baseTickMS
	}
	s.data.Poll.Auto = auto
	if waitMinutes > 0 {
		s.data.Poll.WaitMinutes = waitMinutes
	}
	_ = s.saveLocked()
}
