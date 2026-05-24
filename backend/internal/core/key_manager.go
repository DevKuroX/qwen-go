package core

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"sync"
)

type KeyManager struct {
	mu       sync.RWMutex
	path     string
	keys     []string
}

func NewKeyManager(path string) *KeyManager {
	km := &KeyManager{path: path}
	km.Load()
	return km
}

func (km *KeyManager) Load() {
	km.mu.Lock()
	defer km.mu.Unlock()

	data, err := os.ReadFile(km.path)
	if err != nil {
		if os.IsNotExist(err) {
			km.keys = []string{}
			return
		}
		km.keys = []string{}
		return
	}

	json.Unmarshal(data, &km.keys)
	if km.keys == nil {
		km.keys = []string{}
	}
}

func (km *KeyManager) save() error {
	data, err := json.MarshalIndent(km.keys, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := km.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmpPath, km.path)
}

func (km *KeyManager) Generate() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	key := "devkurox." + hex.EncodeToString(b)

	km.mu.Lock()
	defer km.mu.Unlock()

	km.keys = append(km.keys, key)
	if err := km.save(); err != nil {
		km.keys = km.keys[:len(km.keys)-1]
		return "", err
	}

	return key, nil
}

func (km *KeyManager) Revoke(key string) bool {
	km.mu.Lock()
	defer km.mu.Unlock()

	for i, k := range km.keys {
		if k == key {
			km.keys = append(km.keys[:i], km.keys[i+1:]...)
			km.save()
			return true
		}
	}
	return false
}

func (km *KeyManager) IsValid(key string) bool {
	km.mu.RLock()
	defer km.mu.RUnlock()

	for _, k := range km.keys {
		if k == key {
			return true
		}
	}
	return false
}

func (km *KeyManager) List() []string {
	km.mu.RLock()
	defer km.mu.RUnlock()

	result := make([]string, len(km.keys))
	copy(result, km.keys)
	return result
}
