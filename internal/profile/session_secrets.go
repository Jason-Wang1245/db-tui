package profile

import "sync"

type SessionSecrets struct {
	mu      sync.RWMutex
	secrets map[ID]string
}

func NewSessionSecrets() *SessionSecrets {
	return &SessionSecrets{secrets: make(map[ID]string)}
}

func (s *SessionSecrets) Get(id ID) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	secret, ok := s.secrets[id]
	return secret, ok
}

func (s *SessionSecrets) Set(id ID, secret string) {
	s.mu.Lock()
	s.secrets[id] = secret
	s.mu.Unlock()
}

func (s *SessionSecrets) Delete(id ID) {
	s.mu.Lock()
	delete(s.secrets, id)
	s.mu.Unlock()
}

func (s *SessionSecrets) Clear() {
	s.mu.Lock()
	clear(s.secrets)
	s.mu.Unlock()
}
