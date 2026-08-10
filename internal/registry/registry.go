package registry

import (
	"sync"

	"github.com/hashicorp/yamux"
)

const MaxTunnelsPerUser = 3

type Session struct {
	ID        string
	UserID    string
	Subdomain string
	LocalPort int
	Protocol  string
	Session   *yamux.Session
}

type Registry struct {
	mu          sync.RWMutex
	bySubdomain map[string]*Session
	byID        map[string]*Session
}

func New() *Registry {
	return &Registry{
		bySubdomain: make(map[string]*Session),
		byID:        make(map[string]*Session),
	}
}

func (r *Registry) Exists(subdomain string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.bySubdomain[subdomain]
	return ok
}

func (r *Registry) CountByUser(userID string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n := 0
	for _, s := range r.bySubdomain {
		if s.UserID == userID {
			n++
		}
	}
	return n
}

func (r *Registry) Register(session *Session) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bySubdomain[session.Subdomain] = session
	r.byID[session.ID] = session
}

func (r *Registry) GetBySubdomain(subdomain string) (*Session, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.bySubdomain[subdomain]
	return s, ok
}

func (r *Registry) Remove(id string) *Session {
	r.mu.Lock()
	defer r.mu.Unlock()
	session, ok := r.byID[id]
	if !ok {
		return nil
	}
	delete(r.byID, id)
	delete(r.bySubdomain, session.Subdomain)
	return session
}

func (r *Registry) ListByUser(userID string) []*Session {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []*Session
	for _, s := range r.bySubdomain {
		if s.UserID == userID {
			out = append(out, s)
		}
	}
	return out
}
