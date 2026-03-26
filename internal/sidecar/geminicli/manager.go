package geminicli

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	log "github.com/sirupsen/logrus"
)

// Manager orchestrates gemini-cli-openai sidecar instances.
type Manager struct {
	mu            sync.RWMutex
	cfg           config.GeminiCLIOpenAIConfig
	instances     map[string]*Instance
	procs         map[string]*exec.Cmd
	cancels       map[string]context.CancelFunc
	healthCtx     context.Context
	healthCancel  context.CancelFunc
	submodulePath string
	httpClient    *http.Client
}

// NewManager creates a new sidecar manager.
func NewManager(cfg config.GeminiCLIOpenAIConfig, submodulePath string) *Manager {
	return &Manager{
		cfg:           cfg,
		instances:     make(map[string]*Instance),
		procs:         make(map[string]*exec.Cmd),
		cancels:       make(map[string]context.CancelFunc),
		submodulePath: submodulePath,
		httpClient:    &http.Client{Timeout: 10 * time.Second},
	}
}

// StartAll launches all non-disabled instances and waits for health.
func (m *Manager) StartAll(ctx context.Context) error {
	startTimeout := parseDuration(m.cfg.StartTimeout, 45*time.Second)

	for _, instCfg := range m.cfg.Instances {
		if instCfg.Disabled {
			log.Infof("gemini-cli-openai: instance %s is disabled, skipping", instCfg.ID)
			continue
		}
		inst := &Instance{
			ID:           instCfg.ID,
			Listen:       instCfg.Listen,
			BaseURL:      instCfg.BaseURL,
			CredsFile:    instCfg.CredsFile,
			ProjectID:    instCfg.ProjectID,
			WorkerAPIKey: instCfg.WorkerAPIKey,
			Weight:       instCfg.Weight,
			Status:       StatusPending,
		}
		m.mu.Lock()
		m.instances[inst.ID] = inst
		m.mu.Unlock()

		if m.cfg.Mode == "external" {
			log.Infof("gemini-cli-openai: instance %s in external mode, skipping process start", inst.ID)
			inst.SetStatus(StatusHealthy)
			continue
		}

		if err := m.startInstance(ctx, inst); err != nil {
			log.Errorf("gemini-cli-openai: failed to start instance %s: %v", inst.ID, err)
			inst.LastError = err.Error()
			inst.SetStatus(StatusFailed)
			continue
		}
	}

	// Wait for health on started instances
	deadline := time.After(startTimeout)
	for {
		allReady := true
		m.mu.RLock()
		for _, inst := range m.instances {
			s := inst.GetStatus()
			if s == StatusStarting {
				allReady = false
			}
		}
		m.mu.RUnlock()
		if allReady {
			break
		}
		select {
		case <-deadline:
			log.Warn("gemini-cli-openai: start timeout reached, some instances may not be healthy")
			return nil
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}

	// Start health check loop
	m.startHealthLoop()
	return nil
}

func (m *Manager) startInstance(ctx context.Context, inst *Instance) error {
	inst.SetStatus(StatusStarting)

	// Read credentials file
	credsPath := expandPath(inst.CredsFile)
	credsData, err := os.ReadFile(credsPath)
	if err != nil {
		return fmt.Errorf("read creds file %s: %w", credsPath, err)
	}

	// Parse host:port from Listen
	host, port, err := net.SplitHostPort(inst.Listen)
	if err != nil {
		return fmt.Errorf("parse listen address %s: %w", inst.Listen, err)
	}

	// Build environment
	env := os.Environ()
	env = append(env, fmt.Sprintf("GCP_SERVICE_ACCOUNT=%s", string(credsData)))
	if inst.ProjectID != "" {
		env = append(env, fmt.Sprintf("GEMINI_PROJECT_ID=%s", inst.ProjectID))
	}
	if inst.WorkerAPIKey != "" {
		env = append(env, fmt.Sprintf("OPENAI_API_KEY=%s", inst.WorkerAPIKey))
	}

	// Start wrangler dev process
	procCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(procCtx, "npx", "wrangler", "dev",
		"--host", host,
		"--port", port,
		"--local",
		"--persist-to", ".mf",
	)
	cmd.Dir = m.submodulePath
	cmd.Env = env
	cmd.Stdout = log.StandardLogger().WriterLevel(log.DebugLevel)
	cmd.Stderr = log.StandardLogger().WriterLevel(log.WarnLevel)

	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("start process: %w", err)
	}

	m.mu.Lock()
	m.procs[inst.ID] = cmd
	m.cancels[inst.ID] = cancel
	m.mu.Unlock()

	inst.mu.Lock()
	inst.PID = cmd.Process.Pid
	inst.StartedAt = time.Now()
	inst.mu.Unlock()

	log.Infof("gemini-cli-openai: started instance %s (PID %d) on %s", inst.ID, cmd.Process.Pid, inst.Listen)

	// Monitor process exit in background
	go m.monitorProcess(inst, cmd, cancel)

	// Initial health check with retries
	go func() {
		for i := 0; i < 30; i++ {
			time.Sleep(1 * time.Second)
			if m.checkHealth(inst) {
				inst.SetStatus(StatusHealthy)
				log.Infof("gemini-cli-openai: instance %s is healthy", inst.ID)
				return
			}
		}
		if inst.GetStatus() == StatusStarting {
			inst.SetStatus(StatusUnhealthy)
			log.Warnf("gemini-cli-openai: instance %s failed initial health check", inst.ID)
		}
	}()

	return nil
}

func (m *Manager) monitorProcess(inst *Instance, cmd *exec.Cmd, cancel context.CancelFunc) {
	err := cmd.Wait()
	cancel()

	if inst.GetStatus() == StatusStopped {
		return // intentional stop
	}

	inst.mu.Lock()
	inst.PID = 0
	if err != nil {
		inst.LastError = err.Error()
	}
	inst.mu.Unlock()

	log.Warnf("gemini-cli-openai: instance %s process exited: %v", inst.ID, err)

	// Restart if policy allows
	if m.cfg.RestartPolicy == "on-failure" && inst.RestartCount < m.cfg.MaxRestarts {
		inst.mu.Lock()
		inst.RestartCount++
		inst.mu.Unlock()
		log.Infof("gemini-cli-openai: restarting instance %s (attempt %d/%d)", inst.ID, inst.RestartCount, m.cfg.MaxRestarts)
		if restartErr := m.startInstance(context.Background(), inst); restartErr != nil {
			log.Errorf("gemini-cli-openai: restart failed for %s: %v", inst.ID, restartErr)
			m.enterCooldown(inst)
		}
	} else {
		m.enterCooldown(inst)
	}
}

func (m *Manager) enterCooldown(inst *Instance) {
	cooldown := parseDuration(m.cfg.CooldownOnFail, 2*time.Minute)
	inst.mu.Lock()
	inst.Status = StatusCooldown
	inst.CooldownUntil = time.Now().Add(cooldown)
	inst.mu.Unlock()
	log.Infof("gemini-cli-openai: instance %s entering cooldown until %s", inst.ID, inst.CooldownUntil.Format(time.RFC3339))
}

func (m *Manager) checkHealth(inst *Instance) bool {
	url := strings.TrimRight(inst.BaseURL, "/") + "/models"
	resp, err := m.httpClient.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		inst.mu.Lock()
		inst.LastHealth = time.Now()
		inst.mu.Unlock()
		return true
	}
	return false
}

func (m *Manager) startHealthLoop() {
	interval := parseDuration(m.cfg.HealthcheckInterval, 20*time.Second)
	m.healthCtx, m.healthCancel = context.WithCancel(context.Background())

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-m.healthCtx.Done():
				return
			case <-ticker.C:
				m.mu.RLock()
				instances := make([]*Instance, 0, len(m.instances))
				for _, inst := range m.instances {
					instances = append(instances, inst)
				}
				m.mu.RUnlock()

				for _, inst := range instances {
					status := inst.GetStatus()
					if status == StatusStopped || status == StatusFailed {
						continue
					}
					if inst.InCooldown() {
						continue
					}
					// Check if cooldown expired
					if status == StatusCooldown {
						inst.mu.Lock()
						if time.Now().After(inst.CooldownUntil) {
							inst.Status = StatusUnhealthy
							inst.RestartCount = 0
						}
						inst.mu.Unlock()
						if inst.GetStatus() == StatusCooldown {
							continue
						}
					}

					healthy := m.checkHealth(inst)
					if healthy {
						if status != StatusHealthy {
							inst.SetStatus(StatusHealthy)
							log.Infof("gemini-cli-openai: instance %s recovered, now healthy", inst.ID)
						}
					} else {
						if status == StatusHealthy {
							inst.SetStatus(StatusUnhealthy)
							log.Warnf("gemini-cli-openai: instance %s health check failed", inst.ID)
						}
					}
				}
			}
		}
	}()
}

// StopAll gracefully stops all sidecar instances.
func (m *Manager) StopAll() {
	if m.healthCancel != nil {
		m.healthCancel()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, cancel := range m.cancels {
		log.Infof("gemini-cli-openai: stopping instance %s", id)
		cancel()
		if inst, ok := m.instances[id]; ok {
			inst.SetStatus(StatusStopped)
		}
	}
	// Wait briefly for processes to exit
	for id, cmd := range m.procs {
		if cmd.Process != nil {
			_ = cmd.Process.Signal(os.Interrupt)
			done := make(chan struct{})
			go func() {
				_ = cmd.Wait()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				_ = cmd.Process.Kill()
			}
		}
		delete(m.procs, id)
		delete(m.cancels, id)
	}
}

// HealthyInstances returns instances currently in healthy state.
func (m *Manager) HealthyInstances() []*Instance {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*Instance
	for _, inst := range m.instances {
		if inst.IsHealthy() {
			result = append(result, inst)
		}
	}
	return result
}

// DebugStatus returns debug info for all instances.
func (m *Manager) DebugStatus() []DebugInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]DebugInfo, 0, len(m.instances))
	for _, inst := range m.instances {
		result = append(result, inst.DebugSnapshot())
	}
	return result
}

func parseDuration(s string, fallback time.Duration) time.Duration {
	if s == "" {
		return fallback
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return fallback
	}
	return d
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
