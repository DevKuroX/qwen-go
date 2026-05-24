package core

import "github.com/qwenpi/qwenpi-go/internal/models"

type AccountStore struct {
	db *JSONDatabase
}

func NewAccountStore(path string) *AccountStore {
	return &AccountStore{db: NewJSONDatabase(path, []*models.Account{})}
}

func (s *AccountStore) Load() ([]*models.Account, error) {
	if err := s.db.Load(); err != nil {
		return nil, err
	}
	if data := s.db.Get(); data != nil {
		if accounts, ok := data.([]*models.Account); ok {
			return accounts, nil
		}
	}
	return []*models.Account{}, nil
}

func (s *AccountStore) Save(accounts []*models.Account) error {
	s.db.Set(accounts)
	return s.db.Save()
}

func (s *AccountStore) DB() *JSONDatabase {
	return s.db
}
