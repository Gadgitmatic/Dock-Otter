package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-resty/resty/v2"
	"gopkg.in/yaml.v3"
)

// Build-time variables (set via ldflags)
var (
	version   = "dev"
	buildTime = "unknown"
	gitCommit = "unknown"
)

// Configuration
type Config struct {
	DokployURL       string
	DokployAPIKey    string
	DokployToken     string
	DokploySession   string
	PangolinURL      string
	PangolinAPIKey   string
	PangolinToken    string
	PollInterval     time.Duration
	RetryAttempts    int
	RetryDelay       time.Duration
	RunOnce          bool // For manual execution
	ForceSync        bool // Force re-sync existing resources
}

// Dokploy API types (based on actual API structure)
type DokployProject struct {
	ProjectID    string          `json:"projectId"`
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	Applications []DokployApp    `json:"applications"`
	Compose      []DokployApp    `json:"compose"`
}

type DokployApp struct {
	ApplicationID string `json:"applicationId,omitempty"`
	ComposeID     string `json:"composeId,omitempty"`
	Name          string `json:"name"`
	AppName       string `json:"appName"`
	Description   string `json:"description"`
	Domains       []DokployDomain `json:"domains,omitempty"`
	Port          int    `json:"port,omitempty"`
	Status        string `json:"applicationStatus"`
	ProjectID     string `json:"projectId"`
}

type DokployDomain struct {
	DomainID    string `json:"domainId"`
	Host        string `json:"host"`
	Path        string `json:"path"`
	Port        int    `json:"port"`
	HTTPS       bool   `json:"https"`
	Certificate string `json:"certificate,omitempty"`
}

// Pangolin Blueprint types (simplified for our use case)
type PangolinBlueprint struct {
	ProxyResources []ProxyResource `yaml:"proxy-resources"`
}

type ProxyResource struct {
	Name       string   `yaml:"name"`
	Protocol   string   `yaml:"protocol"`
	FullDomain string   `yaml:"full-domain"`
	SSL        bool     `yaml:"ssl,omitempty"`
	Enabled    bool     `yaml:"enabled"`
	Targets    []Target `yaml:"targets"`
}

type Target struct {
	Hostname string `yaml:"hostname"`
	Port     int    `yaml:"port"`
	Method   string `yaml:"method"`
	Enabled  bool   `yaml:"enabled"`
	Path     string `yaml:"path,omitempty"`
}

// Main application
type DockOtter struct {
	config        *Config
	dokployClient *resty.Client
	pangolinClient *resty.Client
	processedApps map[string]bool
}

func main() {
	// Handle CLI flags
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v":
			fmt.Printf("Dock Otter %s\nBuilt: %s\nCommit: %s\n", version, buildTime, gitCommit)
			os.Exit(0)
		case "--health-check":
			resp, err := http.Get("http://localhost:8080/health")
			if err != nil || resp.StatusCode != 200 {
				os.Exit(1)
			}
			os.Exit(0)
		case "--help", "-h":
			printUsage()
			os.Exit(0)
		}
	}

	// Setup structured logging (2025 best practice)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("🦦 Dock Otter starting up...")

	cfg := loadConfig()
	if err := validateConfig(cfg); err != nil {
		slog.Error("❌ Configuration error", "error", err)
		os.Exit(1)
	}
	
	app := NewDockOtter(cfg)

	// Check for manual execution mode
	if cfg.RunOnce {
		slog.Info("🔄 Running in manual mode (single execution)")
		if err := app.syncApps(); err != nil {
			slog.Error("❌ Manual sync failed", "error", err)
			os.Exit(1)
		}
		slog.Info("✅ Manual sync completed successfully")
		return
	}

	// Start health check server for daemon mode
	go startHealthServer()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		slog.Info("🦦 Shutting down gracefully...")
		cancel()
	}()

	// Start the adapter in daemon mode
	if err := app.Run(ctx); err != nil {
		slog.Error("Adapter failed", "error", err)
		os.Exit(1)
	}

	slog.Info("🦦 Dock Otter stopped")
}

func NewDockOtter(cfg *Config) *DockOtter {
	// Setup Dokploy client with auth and 2025 best practices
	dokployClient := resty.New().
		SetTimeout(30 * time.Second).
		SetRetryCount(2).
		SetRetryWaitTime(1 * time.Second).
		SetRetryMaxWaitTime(5 * time.Second).
		SetHeader("User-Agent", "dock-otter/1.0").
		SetHeader("Accept", "application/json")

	if cfg.DokployAPIKey != "" {
		dokployClient.SetHeader("X-API-Key", cfg.DokployAPIKey)
	}
	if cfg.DokployToken != "" {
		dokployClient.SetHeader("Authorization", "Bearer "+cfg.DokployToken)
	}
	if cfg.DokploySession != "" {
		dokployClient.SetHeader("Cookie", "session="+cfg.DokploySession)
	}

	// Setup Pangolin client with Bearer token auth and best practices
	pangolinClient := resty.New().
		SetTimeout(30 * time.Second).
		SetRetryCount(2).
		SetRetryWaitTime(1 * time.Second).
		SetRetryMaxWaitTime(5 * time.Second).
		SetHeader("User-Agent", "dock-otter/1.0").
		SetHeader("Accept", "application/json")

	if cfg.PangolinToken != "" {
		pangolinClient.SetHeader("Authorization", "Bearer "+cfg.PangolinToken)
	} else if cfg.PangolinAPIKey != "" {
		pangolinClient.SetHeader("Authorization", "Bearer "+cfg.PangolinAPIKey)
	}

	return &DockOtter{
		config:         cfg,
		dokployClient:  dokployClient,
		pangolinClient: pangolinClient,
		processedApps:  make(map[string]bool),
	}
}

func (d *DockOtter) Run(ctx context.Context) error {
	slog.Info("🦦 Starting adapter", "poll_interval", d.config.PollInterval)

	// Log authentication status with structured logging
	dokployAuth := d.getDokployAuthType()
	pangolinAuth := d.getPangolinAuthType()

	slog.Info("🔐 Authentication configured", 
		"dokploy_auth", dokployAuth, 
		"pangolin_auth", pangolinAuth)

	// Test connectivity
	if err := d.testConnectivity(); err != nil {
		slog.Warn("⚠️  Connectivity test failed", "error", err)
	}

	ticker := time.NewTicker(d.config.PollInterval)
	defer ticker.Stop()

	// Initial sync
	if err := d.syncApps(); err != nil {
		slog.Error("❌ Initial sync failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := d.syncApps(); err != nil {
				slog.Error("❌ Sync failed", "error", err)
			}
		}
	}
}

func (d *DockOtter) getDokployAuthType() string {
	if d.config.DokployAPIKey != "" {
		return "API key"
	} else if d.config.DokployToken != "" {
		return "Bearer token"
	} else if d.config.DokploySession != "" {
		return "Session cookie"
	}
	return "none"
}

func (d *DockOtter) getPangolinAuthType() string {
	if d.config.PangolinToken != "" {
		return "Bearer token"
	} else if d.config.PangolinAPIKey != "" {
		return "Bearer token (from API key)"
	}
	return "none"
}

func (d *DockOtter) testConnectivity() error {
	slog.Info("🔍 Testing API connectivity...")

	// Test Dokploy - get projects
	projects, err := d.getDokployProjects()
	if err != nil {
		slog.Error("❌ Dokploy connection failed", "error", err)
		return err
	}
	
	totalApps := 0
	totalDomains := 0
	for _, project := range projects {
		for _, app := range project.Applications {
			totalApps++
			totalDomains += len(app.Domains)
		}
		for _, app := range project.Compose {
			totalApps++
			totalDomains += len(app.Domains)
		}
	}
	slog.Info("✅ Dokploy connected", 
		"projects", len(projects), 
		"apps", totalApps, 
		"domains", totalDomains)

	// Test Pangolin - simple connectivity check
	resp, err := d.pangolinClient.R().Get(d.config.PangolinURL + "/v1/docs")
	if err != nil {
		slog.Warn("⚠️  Pangolin connectivity test failed", "error", err)
	} else {
		slog.Info("✅ Pangolin API accessible", "status", resp.StatusCode())
	}
	
	return nil
}

func (d *DockOtter) syncApps() error {
	slog.Info("🔄 Syncing apps from Dokploy...")

	projects, err := d.getDokployProjects()
	if err != nil {
		return fmt.Errorf("failed to get projects: %w", err)
	}

	processed := 0
	skipped := 0
	errors := 0

	for _, project := range projects {
		slog.Debug("Processing project", "project", project.Name, "id", project.ProjectID)
		
		// Process regular applications
		for _, app := range project.Applications {
			if app.Status != "done" {
				slog.Debug("Skipping app - not running", "app", app.Name, "status", app.Status)
				skipped++
				continue
			}

			if len(app.Domains) == 0 {
				slog.Debug("Skipping app - no domains", "app", app.Name)
				skipped++
				continue
			}

			for _, domain := range app.Domains {
				if err := d.processAppDomain(app, domain); err != nil {
					slog.Error("❌ Failed to process app domain", 
						"app", app.Name, 
						"domain", domain.Host, 
						"error", err)
					errors++
				} else {
					processed++
				}
			}
		}

		// Process compose applications
		for _, app := range project.Compose {
			if app.Status != "done" {
				slog.Debug("Skipping compose - not running", "compose", app.Name, "status", app.Status)
				skipped++
				continue
			}

			if len(app.Domains) == 0 {
				slog.Debug("Skipping compose - no domains", "compose", app.Name)
				skipped++
				continue
			}

			for _, domain := range app.Domains {
				if err := d.processAppDomain(app, domain); err != nil {
					slog.Error("❌ Failed to process compose domain", 
						"compose", app.Name, 
						"domain", domain.Host, 
						"error", err)
					errors++
				} else {
					processed++
				}
			}
		}
	}

	slog.Info("✅ Sync completed", 
		"processed", processed, 
		"skipped", skipped, 
		"errors", errors)
	return nil
}

func (d *DockOtter) processAppDomain(app DokployApp, domain DokployDomain) error {
	resourceName := d.generateResourceName(app.Name, domain.Host)

	// Check if already processed (unless force sync is enabled)
	if !d.config.ForceSync && d.processedApps[resourceName] {
		return nil
	}

	// Validate required fields
	if domain.Host == "" {
		return fmt.Errorf("domain host is empty")
	}

	// Enhanced port resolution logic for 2025 compatibility
	targetPort := d.resolveTargetPort(app, domain)
	if targetPort == 0 {
		return fmt.Errorf("no port available for app %s domain %s", app.Name, domain.Host)
	}

	// Determine target method and hostname with better logic
	targetMethod := "http"
	targetHostname := app.AppName
	
	if domain.HTTPS {
		targetMethod = "https"
	}

	// Handle path-based routing if specified
	targetPath := "/"
	if domain.Path != "" && domain.Path != "/" {
		targetPath = domain.Path
	}

	slog.Info("🔧 Creating Pangolin resource", 
		"domain", domain.Host,
		"app", app.Name,
		"hostname", targetHostname,
		"port", targetPort,
		"method", targetMethod,
		"path", targetPath,
		"ssl", domain.HTTPS)

	// Create enhanced Pangolin blueprint with better domain/port mapping
	blueprint := &PangolinBlueprint{
		ProxyResources: []ProxyResource{
			{
				Name:       resourceName,
				Protocol:   "http",
				FullDomain: domain.Host,
				SSL:        domain.HTTPS,
				Enabled:    true,
				Targets: []Target{
					{
						Hostname: targetHostname,
						Port:     targetPort,
						Method:   targetMethod,
						Enabled:  true,
						Path:     targetPath,
					},
				},
			},
		},
	}

	if err := d.createBlueprintWithRetry(blueprint); err != nil {
		return fmt.Errorf("failed to create blueprint: %w", err)
	}

	d.processedApps[resourceName] = true
	slog.Info("✅ Pangolin resource created", "resource", resourceName, "domain", domain.Host)
	return nil
}

// Enhanced port resolution with fallback logic
func (d *DockOtter) resolveTargetPort(app DokployApp, domain DokployDomain) int {
	// Priority 1: Domain-specific port
	if domain.Port > 0 {
		return domain.Port
	}

	// Priority 2: Application port
	if app.Port > 0 {
		return app.Port
	}

	// Priority 3: Default ports based on protocol
	if domain.HTTPS {
		return 443
	}
	return 80
}

func (d *DockOtter) getDokployProjects() ([]DokployProject, error) {
	// Try multiple endpoints for different Dokploy versions
	endpoints := []string{
		"/api/projects",
		"/api/project/all", 
		"/api/project",
		"/api/applications",
	}
	
	var lastErr error
	for _, endpoint := range endpoints {
		resp, err := d.dokployClient.R().
			SetHeader("Accept", "application/json").
			Get(d.config.DokployURL + endpoint)
		
		if err != nil {
			lastErr = err
			continue
		}
		
		if resp.StatusCode() == 200 {
			var projects []DokployProject
			if err := json.Unmarshal(resp.Body(), &projects); err != nil {
				lastErr = err
				continue
			}
			slog.Info("✅ Found working Dokploy endpoint", "endpoint", endpoint)
			return projects, nil
		}
		
		lastErr = fmt.Errorf("endpoint %s returned status %d", endpoint, resp.StatusCode())
	}

	return nil, fmt.Errorf("all endpoints failed, last error: %w", lastErr)
}

func (d *DockOtter) createBlueprintWithRetry(blueprint *PangolinBlueprint) error {
	var lastErr error

	for attempt := 1; attempt <= d.config.RetryAttempts; attempt++ {
		err := d.createBlueprint(blueprint)
		if err == nil {
			return nil
		}

		lastErr = err
		if attempt < d.config.RetryAttempts {
			slog.Warn("⚠️  Blueprint creation failed, retrying", 
				"attempt", attempt, 
				"retry_in", d.config.RetryDelay, 
				"error", err)
			time.Sleep(d.config.RetryDelay)
		}
	}

	return fmt.Errorf("all %d attempts failed, last error: %w", d.config.RetryAttempts, lastErr)
}

func (d *DockOtter) createBlueprint(blueprint *PangolinBlueprint) error {
	yamlData, err := yaml.Marshal(blueprint)
	if err != nil {
		return fmt.Errorf("failed to marshal blueprint: %w", err)
	}

	// Log the YAML for debugging in debug mode
	slog.Debug("📄 Blueprint YAML", "yaml", string(yamlData))

	// Use the correct Pangolin API endpoint with enhanced error handling
	resp, err := d.pangolinClient.R().
		SetHeader("Content-Type", "application/yaml").
		SetBody(yamlData).
		Post(d.config.PangolinURL + "/v1/blueprints")

	if err != nil {
		return fmt.Errorf("failed to create blueprint: %w", err)
	}

	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		slog.Error("Pangolin API error", 
			"status", resp.StatusCode(), 
			"response", resp.String())
		return fmt.Errorf("pangolin API returned status %d: %s", resp.StatusCode(), resp.String())
	}

	return nil
}

func (d *DockOtter) generateResourceName(appName, domain string) string {
	// Create a safe name for Kubernetes/Pangolin
	name := fmt.Sprintf("%s-%s", appName, domain)
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, ".", "-")
	name = strings.ReplaceAll(name, "_", "-")

	// Ensure it's not too long
	if len(name) > 63 {
		name = name[:63]
	}

	return name
}

func loadConfig() *Config {
	return &Config{
		DokployURL:       getEnv("DOKPLOY_URL", "http://dokploy:3000"),
		DokployAPIKey:    getEnv("DOKPLOY_API_KEY", ""),
		DokployToken:     getEnv("DOKPLOY_TOKEN", ""),
		DokploySession:   getEnv("DOKPLOY_SESSION", ""),
		PangolinURL:      getEnv("PANGOLIN_URL", "http://pangolin:3001"),
		PangolinAPIKey:   getEnv("PANGOLIN_API_KEY", ""),
		PangolinToken:    getEnv("PANGOLIN_TOKEN", ""),
		PollInterval:     getDurationEnv("POLL_INTERVAL", 30*time.Second),
		RetryAttempts:    getIntEnv("RETRY_ATTEMPTS", 3),
		RetryDelay:       getDurationEnv("RETRY_DELAY", 5*time.Second),
		RunOnce:          getBoolEnv("RUN_ONCE", false),
		ForceSync:        getBoolEnv("FORCE_SYNC", false),
	}
}

func validateConfig(cfg *Config) error {
	// Check required URLs
	if cfg.DokployURL == "" {
		return fmt.Errorf("DOKPLOY_URL is required")
	}
	if cfg.PangolinURL == "" {
		return fmt.Errorf("PANGOLIN_URL is required")
	}

	// Check authentication - at least one method for each service
	dokployAuth := cfg.DokployAPIKey != "" || cfg.DokployToken != "" || cfg.DokploySession != ""
	if !dokployAuth {
		slog.Warn("⚠️  No Dokploy authentication configured - API calls may fail")
	}

	pangolinAuth := cfg.PangolinToken != "" || cfg.PangolinAPIKey != ""
	if !pangolinAuth {
		return fmt.Errorf("Pangolin authentication required (PANGOLIN_TOKEN or PANGOLIN_API_KEY)")
	}

	// Validate intervals (only for daemon mode)
	if !cfg.RunOnce {
		if cfg.PollInterval < 5*time.Second {
			return fmt.Errorf("POLL_INTERVAL must be at least 5 seconds")
		}
	}
	
	if cfg.RetryAttempts < 1 || cfg.RetryAttempts > 10 {
		return fmt.Errorf("RETRY_ATTEMPTS must be between 1 and 10")
	}

	// Log configuration for transparency
	slog.Info("Configuration loaded", 
		"dokploy_url", cfg.DokployURL,
		"pangolin_url", cfg.PangolinURL,
		"poll_interval", cfg.PollInterval,
		"retry_attempts", cfg.RetryAttempts,
		"run_once", cfg.RunOnce,
		"force_sync", cfg.ForceSync)

	return nil
}

func startHealthServer() {
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		response := map[string]interface{}{
			"status":     "healthy",
			"service":    "dock-otter",
			"version":    version,
			"build_time": buildTime,
			"git_commit": gitCommit,
			"timestamp":  time.Now().UTC().Format(time.RFC3339),
		}
		json.NewEncoder(w).Encode(response)
	})

	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("# Basic metrics endpoint\ndock_otter_up 1\n"))
	})

	slog.Info("🏥 Health check server starting on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		slog.Error("Health server failed", "error", err)
	}
}

// Helper functions
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getIntEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func getBoolEnv(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func printUsage() {
	fmt.Printf(`🦦 Dock Otter - Dokploy to Pangolin Bridge

Usage:
  dock-otter [flags]

Flags:
  --version, -v     Show version information
  --health-check    Check if service is healthy (for Docker)
  --help, -h        Show this help message

Environment Variables:
  DOKPLOY_URL       Dokploy API endpoint (default: http://dokploy:3000)
  DOKPLOY_API_KEY   Dokploy API key
  PANGOLIN_URL      Pangolin API endpoint (default: http://pangolin:3001)
  PANGOLIN_TOKEN    Pangolin Bearer token (required)
  POLL_INTERVAL     Sync interval (default: 30s)
  RUN_ONCE          Run once and exit (default: false)
  FORCE_SYNC        Force re-sync existing resources (default: false)

Examples:
  # Run as daemon
  dock-otter

  # Run once (manual sync)
  RUN_ONCE=true dock-otter

  # Force sync existing apps
  RUN_ONCE=true FORCE_SYNC=true dock-otter

Version: %s
`, version)
}