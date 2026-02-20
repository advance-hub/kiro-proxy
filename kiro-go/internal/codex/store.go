package codex

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// Credential Codex 凭证
type Credential struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	SessionToken string `json:"sessionToken"`
	AccessToken  string `json:"accessToken,omitempty"`
	Disabled     bool   `json:"disabled,omitempty"`
	UseCount     int64  `json:"useCount,omitempty"`
	ErrorCount   int    `json:"errorCount,omitempty"`
	LastError    string `json:"lastError,omitempty"`
}

// CredentialStore Codex 凭证存储
type CredentialStore struct {
	mu          sync.RWMutex
	credentials []*Credential
	filePath    string
	current     int
}

func NewCredentialStore(path string) *CredentialStore {
	return &CredentialStore{filePath: path}
}

func (s *CredentialStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		s.credentials = nil
		return nil
	}
	var creds []*Credential
	if err := json.Unmarshal(data, &creds); err != nil {
		return fmt.Errorf("解析 codex 凭证失败: %w", err)
	}
	for i, c := range creds {
		if c.ID == 0 {
			c.ID = i + 1
		}
	}
	s.credentials = creds
	return nil
}

func (s *CredentialStore) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := json.MarshalIndent(s.credentials, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0644)
}

func (s *CredentialStore) GetAll() []*Credential {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Credential, len(s.credentials))
	copy(result, s.credentials)
	return result
}

func (s *CredentialStore) GetAllActive() []*Credential {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*Credential
	for _, c := range s.credentials {
		if !c.Disabled {
			result = append(result, c)
		}
	}
	return result
}

func (s *CredentialStore) GetNext() *Credential {
	s.mu.Lock()
	defer s.mu.Unlock()
	var active []*Credential
	for _, c := range s.credentials {
		if !c.Disabled {
			active = append(active, c)
		}
	}
	if len(active) == 0 {
		return nil
	}
	c := active[s.current%len(active)]
	s.current++
	return c
}

func (s *CredentialStore) IncrementUseCount(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.credentials {
		if c.ID == id {
			c.UseCount++
			return
		}
	}
}

func (s *CredentialStore) MarkError(id int, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.credentials {
		if c.ID == id {
			c.ErrorCount++
			c.LastError = errMsg
			return
		}
	}
}

func (s *CredentialStore) MarkDisabled(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.credentials {
		if c.ID == id {
			c.Disabled = true
			return
		}
	}
}

func (s *CredentialStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.credentials)
}

func (s *CredentialStore) ActiveCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, c := range s.credentials {
		if !c.Disabled {
			n++
		}
	}
	return n
}
