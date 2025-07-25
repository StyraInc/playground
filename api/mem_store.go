package api

import (
	"strings"
	"sync"
	"time"
)

// MemoryDataRequestStore is a DataRequestStore backed by memory.
type MemoryDataRequestStore struct {
	store    map[string]DataRequest
	watchers map[string]update
	mu       sync.Mutex
}

// NewMemoryDataRequestStore creates new MemoryDataRequestStores.
func NewMemoryDataRequestStore() *MemoryDataRequestStore {
	return &MemoryDataRequestStore{
		store:    make(map[string]DataRequest),
		watchers: make(map[string]update),
	}
}

// Get a DataRequest (see api.DataRequestStore)
func (s *MemoryDataRequestStore) Get(key *StoreKey, _ *Principal) (DataRequest, bool, error) {
	dr, ok := s.store[key.Id]
	return dr, ok, nil
}

// Put a DataRequest (see api.DataRequestStore)
func (s *MemoryDataRequestStore) Put(key *StoreKey, dr DataRequest, _ *Principal) (*StoreKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store[key.Id] = dr
	if up, ok := s.watchers[key.Id]; ok {
		up.cb(dr)
		delete(s.watchers, key.Id)
		close(up.done)
	}
	return key, nil
}

// List the keys that are set with a given prefix (see api.DataRequestStore)
func (s *MemoryDataRequestStore) List(prefix *StoreKey, _ *Principal) ([]*StoreKey, error) {
	keys := []*StoreKey{}
	for key := range s.store {
		if strings.HasPrefix(key, prefix.Id) {
			keys = append(keys, &StoreKey{Id: key})
		}
	}
	return keys, nil
}

// ListAll the keys that are set (see api.DataRequestStore)
func (s *MemoryDataRequestStore) ListAll(principal *Principal) ([]*StoreKey, error) {
	return s.List(&StoreKey{Id: ""}, principal)
}

// Watch adds a watcher to provide change notifications when the store is changed
func (s *MemoryDataRequestStore) Watch(key *StoreKey, etag string, timeout time.Duration, cb func(DataRequest), _ *Principal) (bool, error) {

	s.mu.Lock()
	dr, ok := s.store[key.Id]
	if !ok {
		s.mu.Unlock()
		return false, nil
	}
	s.mu.Unlock()

	if dr.Etag != etag {
		go cb(dr)
	} else {
		done := make(chan struct{})

		s.mu.Lock()
		s.watchers[key.Id] = update{
			cb:   cb,
			done: done,
		}
		s.mu.Unlock()
		go func() {
			select {
			case <-time.After(timeout):
				s.mu.Lock()
				defer s.mu.Unlock()
				if up, ok := s.watchers[key.Id]; ok {
					dr, _ = s.store[key.Id]
					up.cb(dr)
					delete(s.watchers, key.Id)
				}
			case <-done:
				return
			}
		}()
	}

	return true, nil
}
