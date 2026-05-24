package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/urfave/cli/v2"

	"github.com/qwenpi/qwenpi-go/internal/core"
	"github.com/qwenpi/qwenpi-go/internal/models"
	"github.com/qwenpi/qwenpi-go/internal/server"
)

const (
	defaultAPIPort       = 1440
	defaultDashboardPort = 1441
	pidfileName          = "qwen-go.pid"
)

func main() {
	app := &cli.App{
		Name:    "qwen-go",
		Usage:   "Multi-provider API gateway for Qwen AI",
		Version: "2.0.0",
		Commands: []*cli.Command{
			{
				Name:  "start",
				Usage: "Start the API and dashboard servers",
				Flags: []cli.Flag{
					&cli.IntFlag{Name: "port", Aliases: []string{"p"}, Value: defaultAPIPort, Usage: "API server port"},
					&cli.IntFlag{Name: "dashboard-port", Aliases: []string{"d"}, Value: defaultDashboardPort, Usage: "Dashboard UI port (0 to disable)"},
					&cli.BoolFlag{Name: "dev", Usage: "Serve dashboard from frontend/out/ (no rebuild needed)"},
				},
				Action: func(c *cli.Context) error {
					return runStart(server.Options{
						Port:          c.Int("port"),
						DashboardPort: c.Int("dashboard-port"),
						DevMode:       c.Bool("dev"),
					})
				},
			},
			{
				Name:  "stop",
				Usage: "Stop the running qwen-go server",
				Action: func(c *cli.Context) error {
					return runStop()
				},
			},
			{
				Name:  "restart",
				Usage: "Restart the qwen-go server in background",
				Flags: []cli.Flag{
					&cli.IntFlag{Name: "port", Aliases: []string{"p"}, Value: defaultAPIPort, Usage: "API server port"},
					&cli.IntFlag{Name: "dashboard-port", Aliases: []string{"d"}, Value: defaultDashboardPort, Usage: "Dashboard UI port (0 to disable)"},
					&cli.BoolFlag{Name: "dev", Usage: "Serve dashboard from frontend/out/ (no rebuild needed)"},
				},
				Action: func(c *cli.Context) error {
					return runRestart(RestartOptions{
						Port:          c.Int("port"),
						DashboardPort: c.Int("dashboard-port"),
						DevMode:       c.Bool("dev"),
					})
				},
			},
			{
				Name:  "python",
				Usage: "Manage Python environment",
				Subcommands: []*cli.Command{
					{
						Name:  "setup",
						Usage: "Install Python dependencies (pip + Playwright)",
						Action: func(c *cli.Context) error {
							return runPythonSetup()
						},
					},
					{
						Name:  "check",
						Usage: "Verify Python dependencies are installed",
						Action: func(c *cli.Context) error {
							return runPythonCheck()
						},
					},
				},
			},
			{
				Name:  "account",
				Usage: "Manage accounts",
				Subcommands: []*cli.Command{
					{
						Name:  "list",
						Usage: "List all accounts in the pool",
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "provider", Aliases: []string{"p"}, Usage: "Filter by provider"},
							&cli.StringFlag{Name: "status", Aliases: []string{"s"}, Usage: "Filter by status"},
						},
						Action: func(c *cli.Context) error {
							return runAccountList(c.String("provider"), c.String("status"))
						},
					},
				},
			},
			{
				Name:  "key",
				Usage: "Manage API keys",
				Subcommands: []*cli.Command{
					{
						Name:  "list",
						Usage: "List all API keys",
						Action: func(c *cli.Context) error {
							return runKeyList()
						},
					},
					{
						Name:  "generate",
						Usage: "Generate a new API key",
						Action: func(c *cli.Context) error {
							return runKeyGenerate()
						},
					},
					{
						Name:  "revoke",
						Usage: "Revoke an API key (full key or unique prefix)",
						Action: func(c *cli.Context) error {
							if c.Args().Len() < 1 {
								return fmt.Errorf("usage: qwen-go key revoke <key|prefix>")
							}
							return runKeyRevoke(c.Args().First())
						},
					},
				},
			},
			{
				Name:  "engine",
				Usage: "Manage registration engine",
				Subcommands: []*cli.Command{
					{
						Name:  "list",
						Usage: "List available engines and current selection",
						Action: func(c *cli.Context) error {
							return runEngineList()
						},
					},
					{
						Name:  "set",
						Usage: "Set the registration engine (rod or python)",
						Action: func(c *cli.Context) error {
							if c.Args().Len() < 1 {
								return fmt.Errorf("usage: qwen-go engine set <rod|python>")
							}
							return runEngineSet(c.Args().First())
						},
					},
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

// ── PID file helpers ──

func pidfilePath() string {
	if cfg, err := core.LoadConfig(); err == nil {
		if cfg.AccountsFile != "" {
			return filepath.Join(filepath.Dir(cfg.AccountsFile), pidfileName)
		}
		if cfg.DataDir != "" {
			return filepath.Join(cfg.DataDir, pidfileName)
		}
	}
	return filepath.Join(os.TempDir(), pidfileName)
}

func writePidfile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0644)
}

func readPidfile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("invalid pidfile %s: %w", path, err)
	}
	return pid, nil
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 probes for the process; returns nil if alive.
	return proc.Signal(syscall.Signal(0)) == nil
}

func waitForProcessExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func portFree(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func waitForPortFree(port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if portFree(port) {
			return true
		}
		time.Sleep(150 * time.Millisecond)
	}
	return false
}

// ── start / stop / restart ──

func runStart(opts server.Options) error {
	pf := pidfilePath()
	if pid, err := readPidfile(pf); err == nil {
		if processAlive(pid) {
			return fmt.Errorf("qwen-go already running (PID %d). Use 'qwen-go stop' or 'qwen-go restart'", pid)
		}
		// Stale pidfile from a previous crash — clean up before continuing.
		_ = os.Remove(pf)
	}
	if err := writePidfile(pf); err != nil {
		fmt.Fprintf(os.Stderr, "warn: cannot write pidfile %s: %v\n", pf, err)
	} else {
		defer os.Remove(pf)
	}
	return server.Run(opts)
}

func runStop() error {
	pid, ok, err := terminateRunning(10 * time.Second)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Println("No running qwen-go server found.")
		return nil
	}
	fmt.Printf("Server stopped (PID %d).\n", pid)
	return nil
}

// terminateRunning stops the running server by pidfile (preferred) or by
// scanning processes whose argv matches our binary + "start". Returns the
// PID that was terminated and whether anything was found.
func terminateRunning(timeout time.Duration) (int, bool, error) {
	pf := pidfilePath()

	if pid, err := readPidfile(pf); err == nil {
		if processAlive(pid) {
			proc, _ := os.FindProcess(pid)
			if err := proc.Signal(syscall.SIGTERM); err != nil {
				return 0, false, fmt.Errorf("failed to signal PID %d: %w", pid, err)
			}
			if !waitForProcessExit(pid, timeout) {
				_ = proc.Signal(syscall.SIGKILL)
				_ = waitForProcessExit(pid, 2*time.Second)
			}
			_ = os.Remove(pf)
			return pid, true, nil
		}
		// Stale pidfile.
		_ = os.Remove(pf)
	}

	// Fallback: search /proc for matching processes (Linux). Match must be
	// tighter than `pkill -f "qwen-go start"` to avoid killing unrelated
	// processes that happen to contain that string.
	pid := findRunningPID()
	if pid == 0 {
		return 0, false, nil
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return 0, false, fmt.Errorf("PID %d not signalable: %w", pid, err)
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return 0, false, fmt.Errorf("failed to signal PID %d: %w", pid, err)
	}
	if !waitForProcessExit(pid, timeout) {
		_ = proc.Signal(syscall.SIGKILL)
		_ = waitForProcessExit(pid, 2*time.Second)
	}
	return pid, true, nil
}

// findRunningPID scans /proc for a process whose executable is the same as
// ours (matched via /proc/<pid>/exe symlink) and whose argv contains "start".
// Falls back to a basename match using our own binary name when the exe link
// is unreadable. Skips the current process so we don't kill ourselves.
func findRunningPID() int {
	self := os.Getpid()
	selfExe, _ := os.Executable()
	selfExeReal, _ := filepath.EvalSymlinks(selfExe)
	selfBase := filepath.Base(selfExe)

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == self {
			continue
		}

		// Argv check first — we only ever want `start` processes.
		data, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		if err != nil || len(data) == 0 {
			continue
		}
		argv := strings.Split(strings.TrimRight(string(data), "\x00"), "\x00")
		if len(argv) < 2 {
			continue
		}
		hasStart := false
		for _, a := range argv[1:] {
			if a == "start" {
				hasStart = true
				break
			}
		}
		if !hasStart {
			continue
		}

		// Prefer matching via the exe symlink — robust against rename.
		if selfExeReal != "" {
			if exe, err := os.Readlink(filepath.Join("/proc", e.Name(), "exe")); err == nil {
				if exeReal, err2 := filepath.EvalSymlinks(exe); err2 == nil && exeReal == selfExeReal {
					return pid
				}
				if exe == selfExeReal {
					return pid
				}
			}
		}

		// Fallback: argv[0] basename matches our own basename.
		if filepath.Base(argv[0]) == selfBase {
			return pid
		}
	}
	return 0
}

type RestartOptions struct {
	Port          int
	DashboardPort int
	DevMode       bool
}

func runRestart(opts RestartOptions) error {
	if _, _, err := terminateRunning(10 * time.Second); err != nil {
		return fmt.Errorf("stop failed: %w", err)
	}

	port := opts.Port
	if port == 0 {
		port = defaultAPIPort
	}
	if !waitForPortFree(port, 10*time.Second) {
		return fmt.Errorf("port %d still in use after old server shutdown — aborting restart", port)
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot find binary path: %w", err)
	}

	workDir := findBackendDir()
	if workDir == "" {
		return fmt.Errorf("cannot find backend/ directory with .env")
	}

	logFile := filepath.Join(os.TempDir(), "qwen-go.log")
	f, err := os.Create(logFile)
	if err != nil {
		return fmt.Errorf("cannot create log file: %w", err)
	}
	f.Close()
	f, err = os.OpenFile(logFile, os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("cannot open log file: %w", err)
	}
	defer f.Close()

	args := []string{"start"}
	if opts.Port != defaultAPIPort {
		args = append(args, "--port", strconv.Itoa(opts.Port))
	}
	if opts.DashboardPort != defaultDashboardPort {
		args = append(args, "--dashboard-port", strconv.Itoa(opts.DashboardPort))
	}
	if opts.DevMode {
		args = append(args, "--dev")
	}

	cmd := exec.Command("setsid", append([]string{exe}, args...)...)
	cmd.Dir = workDir
	cmd.Stdout = f
	cmd.Stderr = f
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	// Give the child a moment to either bind the port or crash; report status
	// rather than silently leaving the user to check logs.
	bound := false
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		if !portFree(port) {
			bound = true
			break
		}
		// Detect early child exit.
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			return fmt.Errorf("server exited immediately; check %s", logFile)
		}
	}

	if !bound {
		fmt.Printf("Server launched (PID %d) but port %d is not yet bound. Check %s\n", cmd.Process.Pid, port, logFile)
	} else {
		fmt.Printf("Server restarted (PID %d) on port %d.\n", cmd.Process.Pid, port)
	}
	fmt.Printf("Logs: %s\n", logFile)
	return nil
}

// ── Python ──

func runPythonSetup() error {
	script, err := findPythonScript("setup.sh")
	if err != nil {
		return err
	}
	cmd := exec.Command("bash", script)
	cmd.Dir = filepath.Dir(script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runPythonCheck() error {
	cmd := exec.Command("python3", "-c",
		"import curl_cffi; import playwright; import httpx; import pydantic; print('All Python dependencies OK')")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		fmt.Println("Missing Python dependencies. Run: qwen-go python setup")
	}
	return err
}

// findPythonScript searches for python/<name> relative to the binary, the
// CWD, and well-known project layouts.
func findPythonScript(name string) (string, error) {
	var candidates []string
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "python", name),
			filepath.Join(exeDir, "backend", "python", name),
			filepath.Join(filepath.Dir(exeDir), "python", name),
			filepath.Join(filepath.Dir(exeDir), "backend", "python", name),
		)
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(cwd, "python", name),
			filepath.Join(cwd, "backend", "python", name),
		)
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("%s not found in python/ or backend/python/", name)
}

// ── Account ──

func runAccountList(providerFilter, statusFilter string) error {
	config, err := core.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	data, err := os.ReadFile(config.GetAccountsFile())
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No accounts found.")
			return nil
		}
		return fmt.Errorf("failed to read accounts: %w", err)
	}

	var accounts []*models.Account
	if err := json.Unmarshal(data, &accounts); err != nil {
		return fmt.Errorf("failed to parse accounts: %w", err)
	}

	if len(accounts) == 0 {
		fmt.Println("No accounts found.")
		return nil
	}

	providerFilter = strings.ToLower(strings.TrimSpace(providerFilter))
	statusFilter = strings.ToUpper(strings.TrimSpace(statusFilter))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "Email\tProvider\tStatus\tInflight\tScore\tError")
	fmt.Fprintln(w, "-----\t--------\t------\t--------\t-----\t-----")

	shown := 0
	for _, acc := range accounts {
		if providerFilter != "" && strings.ToLower(acc.Provider) != providerFilter {
			continue
		}
		if statusFilter != "" && strings.ToUpper(string(acc.Status)) != statusFilter {
			continue
		}

		errMsg := acc.LastError
		if len(errMsg) > 40 {
			errMsg = errMsg[:40] + "..."
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%.0f\t%s\n",
			acc.Email, acc.Provider, acc.Status, acc.Inflight, acc.Score, errMsg)
		shown++
	}
	w.Flush()

	if providerFilter != "" || statusFilter != "" {
		fmt.Printf("\nShown: %d / %d accounts\n", shown, len(accounts))
	} else {
		fmt.Printf("\nTotal: %d accounts\n", len(accounts))
	}
	return nil
}

// ── Key ──

func runKeyList() error {
	config, err := core.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	km := core.NewKeyManager(config.GetAPIKeysFile())
	keys := km.List()

	if len(keys) == 0 {
		fmt.Println("No API keys found.")
		return nil
	}

	fmt.Println("API Keys:")
	for i, k := range keys {
		fmt.Printf("  %d. %s\n", i+1, k)
	}
	fmt.Printf("\nTotal: %d keys\n", len(keys))
	return nil
}

func runKeyGenerate() error {
	config, err := core.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	km := core.NewKeyManager(config.GetAPIKeysFile())
	key, err := km.Generate()
	if err != nil {
		return fmt.Errorf("failed to generate key: %w", err)
	}

	fmt.Println("Generated API key:")
	fmt.Println("  " + key)
	return nil
}

func runKeyRevoke(input string) error {
	config, err := core.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	km := core.NewKeyManager(config.GetAPIKeysFile())

	// Try exact match first.
	if km.Revoke(input) {
		fmt.Println("Key revoked successfully.")
		return nil
	}

	// Then try unique-prefix match.
	matches := []string{}
	for _, k := range km.List() {
		if strings.HasPrefix(k, input) {
			matches = append(matches, k)
		}
	}
	switch len(matches) {
	case 0:
		return fmt.Errorf("key not found")
	case 1:
		if !km.Revoke(matches[0]) {
			return fmt.Errorf("key not found")
		}
		fmt.Println("Key revoked successfully.")
		return nil
	default:
		return fmt.Errorf("prefix %q matches %d keys — provide more characters", input, len(matches))
	}
}

// ── Engine ──

func runEngineList() error {
	config, err := core.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	available := []string{"rod", "python"}

	fmt.Println("Registration Engines:")
	for _, e := range available {
		mark := " "
		if e == config.RegistrationEngine {
			mark = "*"
		}
		fmt.Printf("  %s %s\n", mark, e)
	}
	fmt.Printf("\nCurrent: %s\n", config.RegistrationEngine)
	return nil
}

func runEngineSet(engine string) error {
	engine = strings.ToLower(engine)
	if engine != "rod" && engine != "python" {
		return fmt.Errorf("invalid engine: %s (valid: rod, python)", engine)
	}

	envPath := findEnvFile()
	if envPath == "" {
		return fmt.Errorf(".env file not found")
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		return fmt.Errorf("failed to read .env: %w", err)
	}

	content := string(data)
	var lines []string
	if content != "" {
		// Split, but trim a single trailing newline so we don't introduce a
		// blank line on every write.
		content = strings.TrimRight(content, "\n")
		lines = strings.Split(content, "\n")
	}

	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "REGISTRATION_ENGINE=") || strings.HasPrefix(trimmed, "REGISTRATION_ENGINE =") {
			lines[i] = fmt.Sprintf("REGISTRATION_ENGINE=%s", engine)
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, fmt.Sprintf("REGISTRATION_ENGINE=%s", engine))
	}

	if err := os.WriteFile(envPath, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		return fmt.Errorf("failed to write .env: %w", err)
	}

	fmt.Printf("Registration engine set to: %s\n", engine)
	fmt.Println("Run `qwen-go restart` for the change to take effect.")
	return nil
}

func findEnvFile() string {
	candidates := []string{".env", "backend/.env", "../.env"}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func findBackendDir() string {
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		if _, err := os.Stat(filepath.Join(dir, ".env")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if _, err := os.Stat(filepath.Join(parent, ".env")); err == nil {
			return parent
		}
	}
	cwd, _ := os.Getwd()
	if _, err := os.Stat(filepath.Join(cwd, ".env")); err == nil {
		return cwd
	}
	if _, err := os.Stat(filepath.Join(cwd, "backend", ".env")); err == nil {
		return filepath.Join(cwd, "backend")
	}
	return ""
}
