package core

import "github.com/qwenpi/qwenpi-go/internal/models"

type ProxyStore struct {
	db *JSONDatabase
}

func NewProxyStore(path string) *ProxyStore {
	return &ProxyStore{db: NewJSONDatabase(path, []*models.Proxy{})}
}

func (s *ProxyStore) Load() ([]*models.Proxy, error) {
	if err := s.db.Load(); err != nil {
		return nil, err
	}
	if data := s.db.Get(); data != nil {
		if proxies, ok := data.([]*models.Proxy); ok {
			return proxies, nil
		}
	}
	return []*models.Proxy{}, nil
}

func (s *ProxyStore) Save(proxies []*models.Proxy) error {
	s.db.Set(proxies)
	return s.db.Save()
}

func (s *ProxyStore) DB() *JSONDatabase {
	return s.db
}
