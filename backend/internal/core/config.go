package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"github.com/spf13/viper"
)

type Config struct {
	AdminKey           string `mapstructure:"ADMIN_KEY"`
	Port               int    `mapstructure:"PORT"`
	Workers            int    `mapstructure:"WORKERS"`
	EngineMode         string `mapstructure:"ENGINE_MODE"`
	BrowserPoolSize    int    `mapstructure:"BROWSER_POOL_SIZE"`
	MaxInflight        int    `mapstructure:"MAX_INFLIGHT"`
	AccountMinInterval int    `mapstructure:"ACCOUNT_MIN_INTERVAL_MS"`
	RequestJitterMin   int    `mapstructure:"REQUEST_JITTER_MIN_MS"`
	RequestJitterMax   int    `mapstructure:"REQUEST_JITTER_MAX_MS"`
	MaxRetries         int    `mapstructure:"MAX_RETRIES"`
	AutoReplenish      bool   `mapstructure:"AUTO_REPLENISH"`
	ReplenishTarget    int    `mapstructure:"REPLENISH_TARGET"`
	RegistrationEngine string `mapstructure:"REGISTRATION_ENGINE"`
	
	DataDir        string
	AccountsFile   string `mapstructure:"ACCOUNTS_FILE"`
	UsersFile      string `mapstructure:"USERS_FILE"`
	CapturesFile   string `mapstructure:"CAPTURES_FILE"`
	ConfigFile     string `mapstructure:"CONFIG_FILE"`
}

var GlobalConfig *Config

func LoadConfig() (*Config, error) {
	baseDir, err := resolveBaseDir()
	if err != nil {
		return nil, fmt.Errorf("resolve base dir: %w", err)
	}
	configDir := resolveConfigDir(baseDir)

	viper.SetConfigFile(filepath.Join(configDir, ".env"))
	viper.SetConfigType("env")
	viper.AutomaticEnv()
	
	viper.SetDefault("PORT", 1440)
	viper.SetDefault("WORKERS", 1)
	viper.SetDefault("ENGINE_MODE", "hybrid")
	viper.SetDefault("BROWSER_POOL_SIZE", 2)
	viper.SetDefault("MAX_INFLIGHT", 1)
	viper.SetDefault("MAX_RETRIES", 2)
	viper.SetDefault("AUTO_REPLENISH", false)
	viper.SetDefault("REPLENISH_TARGET", 30)
	viper.SetDefault("REGISTRATION_ENGINE", "python")
	
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config: %w", err)
		}
	}
	
	config := &Config{
		AdminKey:           viper.GetString("ADMIN_KEY"),
		Port:               viper.GetInt("PORT"),
		Workers:            viper.GetInt("WORKERS"),
		EngineMode:         viper.GetString("ENGINE_MODE"),
		BrowserPoolSize:    viper.GetInt("BROWSER_POOL_SIZE"),
		MaxInflight:        viper.GetInt("MAX_INFLIGHT"),
		AccountMinInterval: viper.GetInt("ACCOUNT_MIN_INTERVAL_MS"),
		RequestJitterMin:   viper.GetInt("REQUEST_JITTER_MIN_MS"),
		RequestJitterMax:   viper.GetInt("REQUEST_JITTER_MAX_MS"),
		MaxRetries:         viper.GetInt("MAX_RETRIES"),
		AutoReplenish:      viper.GetBool("AUTO_REPLENISH"),
		ReplenishTarget:    viper.GetInt("REPLENISH_TARGET"),
		RegistrationEngine: viper.GetString("REGISTRATION_ENGINE"),
		DataDir:            filepath.Join(baseDir, "data"),
		AccountsFile:       viper.GetString("ACCOUNTS_FILE"),
		UsersFile:          viper.GetString("USERS_FILE"),
		CapturesFile:       viper.GetString("CAPTURES_FILE"),
		ConfigFile:         viper.GetString("CONFIG_FILE"),
	}
	
	if config.AccountsFile == "" {
		config.AccountsFile = filepath.Join(config.DataDir, "accounts.json")
	}
	if config.UsersFile == "" {
		config.UsersFile = filepath.Join(config.DataDir, "users.json")
	}
	
	if config.AdminKey == "" {
		config.AdminKey = "123456"
	}
	
	GlobalConfig = config
	return config, nil
}

func resolveBaseDir() (string, error) {
	if baseDir := os.Getenv("QWEN_GO_BASE_DIR"); baseDir != "" {
		return filepath.Abs(baseDir)
	}

	exePath, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exePath)
		if rootDir, ok := detectProjectRoot(exeDir); ok {
			return rootDir, nil
		}
		if rootDir, ok := detectProjectRoot(filepath.Dir(exeDir)); ok {
			return rootDir, nil
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if rootDir, ok := detectProjectRoot(wd); ok {
		return rootDir, nil
	}
	if rootDir, ok := detectProjectRoot(filepath.Dir(wd)); ok {
		return rootDir, nil
	}

	return wd, nil
}

func resolveConfigDir(baseDir string) string {
	backendDir := filepath.Join(baseDir, "backend")
	if fileExists(filepath.Join(backendDir, ".env")) {
		return backendDir
	}
	if fileExists(filepath.Join(baseDir, ".env")) {
		return baseDir
	}
	return backendDir
}

func detectProjectRoot(dir string) (string, bool) {
	if dir == "" {
		return "", false
	}
	if fileExists(filepath.Join(dir, "backend", "python", "register.py")) && dirExists(filepath.Join(dir, "data")) {
		return dir, true
	}
	return "", false
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func (c *Config) GetAccountsFile() string {
	if c.AccountsFile != "" {
		return c.AccountsFile
	}
	return filepath.Join(c.DataDir, "accounts.json")
}

func (c *Config) GetAPIKeysFile() string {
	if c.AccountsFile != "" {
		return filepath.Join(filepath.Dir(c.AccountsFile), "api_keys.json")
	}
	return filepath.Join(c.DataDir, "api_keys.json")
}

func (c *Config) GetSettingsFile() string {
	if c.AccountsFile != "" {
		return filepath.Join(filepath.Dir(c.AccountsFile), "settings.json")
	}
	return filepath.Join(c.DataDir, "settings.json")
}

func (c *Config) GetAuthFile() string {
	if c.AccountsFile != "" {
		return filepath.Join(filepath.Dir(c.AccountsFile), "auth.json")
	}
	return filepath.Join(c.DataDir, "auth.json")
}

func (c *Config) GetUsageFile() string {
	if c.AccountsFile != "" {
		return filepath.Join(filepath.Dir(c.AccountsFile), "usage.jsonl")
	}
	return filepath.Join(c.DataDir, "usage.jsonl")
}

func (c *Config) GetUsageDB() string {
	if c.AccountsFile != "" {
		return filepath.Join(filepath.Dir(c.AccountsFile), "qwen-go.db")
	}
	return filepath.Join(c.DataDir, "qwen-go.db")
}

func (c *Config) GetUsersFile() string {
	if c.UsersFile != "" {
		return c.UsersFile
	}
	return filepath.Join(c.DataDir, "users.json")
}

func (c *Config) GetProxiesFile() string {
	if c.AccountsFile != "" {
		dir := filepath.Dir(c.AccountsFile)
		return filepath.Join(dir, "proxies.json")
	}
	return filepath.Join(c.DataDir, "proxies.json")
}

func (c *Config) GetProviderConfigsFile() string {
	if c.AccountsFile != "" {
		return filepath.Join(filepath.Dir(c.AccountsFile), "provider_configs.json")
	}
	return filepath.Join(c.DataDir, "provider_configs.json")
}

func (c *Config) GetHistoryImagesFile() string {
	if c.AccountsFile != "" {
		return filepath.Join(filepath.Dir(c.AccountsFile), "history_images.json")
	}
	return filepath.Join(c.DataDir, "history_images.json")
}

func (c *Config) GetHistoryVideosFile() string {
	if c.AccountsFile != "" {
		return filepath.Join(filepath.Dir(c.AccountsFile), "history_videos.json")
	}
	return filepath.Join(c.DataDir, "history_videos.json")
}

func (c *Config) GetPythonRegisterScript() string {
	return filepath.Join(filepath.Dir(c.DataDir), "backend", "python", "register.py")
}

type JSONDatabase struct {
	mu       sync.RWMutex
	path     string
	data     interface{}
}

func NewJSONDatabase(path string, defaultData interface{}) *JSONDatabase {
	return &JSONDatabase{
		path: path,
		data: defaultData,
	}
}

func (db *JSONDatabase) Load() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	
	data, err := os.ReadFile(db.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	
	if db.data == nil {
		return errors.New("json database target is nil")
	}

	target := reflectNew(db.data)
	if err := json.Unmarshal(data, target); err != nil {
		return err
	}

	db.data = derefValue(target)
	return nil
}

func (db *JSONDatabase) Save() error {
	db.mu.RLock()
	defer db.mu.RUnlock()
	
	data, err := json.MarshalIndent(db.data, "", "  ")
	if err != nil {
		return err
	}
	
	tmpPath := db.path + ".tmp"
	if err := os.MkdirAll(filepath.Dir(db.path), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	
	return os.Rename(tmpPath, db.path)
}

func (db *JSONDatabase) Get() interface{} {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.data
}

func (db *JSONDatabase) Set(data interface{}) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.data = data
}

func reflectNew(data interface{}) interface{} {
	v := reflect.ValueOf(data)
	if v.Kind() == reflect.Ptr {
		return reflect.New(v.Elem().Type()).Interface()
	}
	return reflect.New(v.Type()).Interface()
}

func derefValue(data interface{}) interface{} {
	v := reflect.ValueOf(data)
	if v.Kind() == reflect.Ptr {
		return v.Elem().Interface()
	}
	return data
}
