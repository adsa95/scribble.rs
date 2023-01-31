package twitch

import (
	"github.com/scribble-rs/scribble.rs/auth"
	"sync"
	"time"
)

type TokenSet struct {
	AccessToken          string
	RefreshToken         string
	FetchedAt            time.Time
	AccessTokenExpiresAt time.Time
	Scopes               []string
}

func (t *TokenSet) HasScope(scope string) bool {
	if len(t.Scopes) == 0 {
		return false
	}

	for _, s := range t.Scopes {
		if s == scope {
			return true
		}
	}

	return false
}

type TokenStore interface {
	Get(user *auth.User) (*TokenSet, error)
	Set(user *auth.User, tokens *TokenSet) error
}

func NewMemoryTokenStore() TokenStore {
	return &memoryTokenStore{
		tokens: []MemoryTokenStoreEntry{},
		mutex:  &sync.Mutex{},
	}
}

type memoryTokenStore struct {
	tokens []MemoryTokenStoreEntry
	mutex  *sync.Mutex
}

type MemoryTokenStoreEntry struct {
	Id     string
	Tokens *TokenSet
}

func (s *memoryTokenStore) Get(user *auth.User) (*TokenSet, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	for _, entry := range s.tokens {
		if entry.Id == user.Id {
			return entry.Tokens, nil
		}
	}

	return nil, nil
}

func (s *memoryTokenStore) Set(user *auth.User, tokens *TokenSet) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	for i, entry := range s.tokens {
		if entry.Id == user.Id {
			s.tokens[i].Tokens = tokens
			return nil
		}
	}

	s.tokens = append(s.tokens, MemoryTokenStoreEntry{
		Id:     user.Id,
		Tokens: tokens,
	})

	return nil
}
