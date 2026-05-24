package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/qwenpi/qwenpi-go/internal/models"
	"go.uber.org/zap"
)

type ProxyEnvProvider func() (enabled bool, proxyURL, username, password string)

type PythonEngine struct {
	pythonPath    string
	scriptPath    string
	mu            sync.Mutex
	logger        *zap.Logger
	proxyProvider ProxyEnvProvider
}

func NewPythonEngine(pythonPath, scriptPath string) *PythonEngine {
	if pythonPath == "" {
		pythonPath = "python3"
	}
	return &PythonEngine{
		pythonPath: pythonPath,
		scriptPath: scriptPath,
		logger:     zap.L(),
	}
}

func (e *PythonEngine) SetProxyProvider(provider ProxyEnvProvider) {
	e.proxyProvider = provider
}

func (e *PythonEngine) Name() string {
	return "python"
}

func (e *PythonEngine) Register(ctx context.Context, req models.RegistrationRequest, onLog func(string)) (*models.Account, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	timeout := time.Duration(req.Count*60+60) * time.Second
	if timeout > 5*time.Minute {
		timeout = 5 * time.Minute
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.pythonPath, e.scriptPath, "--json-rpc")

	if e.proxyProvider != nil {
		enabled, proxyURL, username, password := e.proxyProvider()
		cmd.Env = append(os.Environ(),
			fmt.Sprintf("PROXY_ENABLED=%t", enabled),
			fmt.Sprintf("PROXY_URL=%s", proxyURL),
			fmt.Sprintf("PROXY_USERNAME=%s", username),
			fmt.Sprintf("PROXY_PASSWORD=%s", password),
		)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start python process: %w", err)
	}

	if err := json.NewEncoder(stdin).Encode(req); err != nil {
		cmd.Process.Kill()
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	stdin.Close()

	var resp models.RegistrationResponse
	decoder := json.NewDecoder(stdout)
	if err := decoder.Decode(&resp); err != nil {
		errOutput, _ := io.ReadAll(stderr)
		cmd.Process.Kill()
		return nil, fmt.Errorf("failed to decode response: %w, stderr: %s", err, string(errOutput))
	}

	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("python process failed: %w", err)
	}

	if !resp.Success || len(resp.Accounts) == 0 {
		errMsg := "registration returned no accounts"
		if resp.Error != "" {
			errMsg = resp.Error
		}
		return nil, fmt.Errorf("registration failed: %s", errMsg)
	}

	var account models.Account
	if err := json.Unmarshal(resp.Accounts[0], &account); err != nil {
		return nil, fmt.Errorf("failed to parse account: %w", err)
	}

	if onLog != nil {
		onLog(fmt.Sprintf("Account registered: %s", account.Email))
	}

	e.logger.Info("registration completed via python",
		zap.String("email", account.Email),
	)

	return &account, nil
}

func CheckPythonDependencies() error {
	cmd := exec.Command("python3", "-c", "import playwright; import curl_cffi")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("missing Python dependencies: pip install playwright curl_cffi")
	}
	return nil
}
