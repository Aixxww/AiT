package main

import (
	aitiagent "github.com/Aixxww/AiT/agent"
	"github.com/Aixxww/AiT/api"
	"github.com/Aixxww/AiT/auth"
	"github.com/Aixxww/AiT/backtest"
	"github.com/Aixxww/AiT/config"
	"github.com/Aixxww/AiT/crypto"
	"github.com/Aixxww/AiT/logger"
	"github.com/Aixxww/AiT/manager"
	"github.com/Aixxww/AiT/mcp"
	_ "github.com/Aixxww/AiT/mcp/payment"
	_ "github.com/Aixxww/AiT/mcp/provider"
	"github.com/Aixxww/AiT/store"
	"github.com/Aixxww/AiT/telegram"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/joho/godotenv"
)

func main() {
	// Load .env environment variables
	_ = godotenv.Load()

	// Initialize logger
	logger.Init(nil)

	logger.Info("╔════════════════════════════════════════════════════════════╗")
	logger.Info("║           🚀 AIT - AI-Powered Trading System              ║")
	logger.Info("╚════════════════════════════════════════════════════════════╝")

	// Initialize global configuration (loaded from .env)
	config.Init()
	cfg := config.Get()
	logger.Info("✅ Configuration loaded")

	// Initialize encryption service BEFORE database (so EncryptedString can decrypt on read)
	logger.Info("🔐 Initializing encryption service...")
	cryptoService, err := crypto.NewCryptoService()
	if err != nil {
		logger.Fatalf("❌ Failed to initialize encryption service: %v", err)
	}
	crypto.SetGlobalCryptoService(cryptoService)
	logger.Info("✅ Encryption service initialized successfully")

	// Initialize database from configuration
	// For backward compatibility: command line arg overrides config (SQLite only)
	if len(os.Args) > 1 {
		cfg.DBPath = os.Args[1]
	}
	// Ensure data directory exists (for SQLite)
	if cfg.DBType == "sqlite" {
		if dir := filepath.Dir(cfg.DBPath); dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				logger.Errorf("Failed to create data directory: %v", err)
			}
		}
	}

	logger.Infof("📋 Initializing database (%s)...", cfg.DBType)
	dbType := store.DBTypeSQLite
	if cfg.DBType == "postgres" {
		dbType = store.DBTypePostgres
	}
	st, err := store.NewWithConfig(store.DBConfig{
		Type:     dbType,
		Path:     cfg.DBPath,
		Host:     cfg.DBHost,
		Port:     cfg.DBPort,
		User:     cfg.DBUser,
		Password: cfg.DBPassword,
		DBName:   cfg.DBName,
		SSLMode:  cfg.DBSSLMode,
	})
	if err != nil {
		logger.Fatalf("❌ Failed to initialize database: %v", err)
	}
	defer st.Close()
	backtest.UseDatabaseWithType(st.DB(), st.DBType() == store.DBTypePostgres)

	// Set JWT secret
	auth.SetJWTSecret(cfg.JWTSecret)
	logger.Info("🔑 JWT secret configured")

	// Legacy WebSocket market monitor is not started from main.
	// Market data is fetched from Binance first with CoinAnk fallback; SnapshotStore/WS
	// data sources are managed by trader datafetch components.
	// go market.NewWSMonitor(150).Start(nil)
	// logger.Info("📊 WebSocket market monitor started")
	// time.Sleep(500 * time.Millisecond)
	logger.Info("📊 Market data source: Binance first, CoinAnk fallback; trader SnapshotStore/WS managed separately")

	// Create TraderManager
	traderManager := manager.NewTraderManager()

	// Create BacktestManager
	mcpClient := newSharedMCPClient()
	backtestManager := backtest.NewManager(mcpClient)
	if err := backtestManager.RestoreRuns(); err != nil {
		logger.Warnf("⚠️ Failed to restore backtest history: %v", err)
	}

	// Load all traders from database to memory (may auto-start traders with IsRunning=true)
	if err := traderManager.LoadTradersFromStore(st); err != nil {
		logger.Fatalf("❌ Failed to load traders: %v", err)
	}

	// Display loaded trader information
	traders, err := st.Trader().List("default")
	if err != nil {
		logger.Fatalf("❌ Failed to get trader list: %v", err)
	}

	logger.Info("🤖 AI Trader Configurations in Database:")
	if len(traders) == 0 {
		logger.Info("  (No trader configurations, please create via Web interface)")
	} else {
		for _, t := range traders {
			status := "❌ Stopped"
			if t.IsRunning {
				status = "✅ Running"
			}
			idShort := t.ID
			if len(idShort) > 8 {
				idShort = idShort[:8]
			}
			logger.Infof("  • %s [%s] %s - AI Model: %s, Exchange: %s",
				t.Name, idShort, status, t.AIModelID, t.ExchangeID)
		}
	}

	// Start API server
	server := api.NewServer(traderManager, st, cryptoService, backtestManager, cfg.APIServerPort)

	// Create hot-reload channel for Telegram bot; wire it to the API server
	// so that POST /api/telegram can trigger a bot restart when the token changes.
	telegramReloadCh := make(chan struct{}, 1)
	server.SetTelegramReloadCh(telegramReloadCh)

	go func() {
		if err := server.Start(); err != nil {
			logger.Fatalf("❌ Failed to start API server: %v", err)
		}
	}()

	// Start the AITi web agent on top of the current dev branch services.
	aitiAgent := aitiagent.New(traderManager, st, nil, slog.Default())
	aitiAgent.Start()
	defer aitiAgent.Stop()

	agentWeb := aitiagent.NewWebHandler(aitiAgent, slog.Default())
	server.RegisterAgentHandler(agentWeb)

	// Start Telegram bot (if TELEGRAM_BOT_TOKEN is configured)
	go telegram.Start(cfg, st, telegramReloadCh)

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("✅ System started successfully, waiting for trading commands...")
	logger.Info("📌 Tip: Use Ctrl+C to stop the system")

	<-quit
	logger.Info("📴 Shutdown signal received, closing system...")

	if err := server.Shutdown(); err != nil {
		logger.Warnf("⚠️ HTTP server shutdown error: %v", err)
	}
	logger.Info("✅ HTTP server stopped")

	// aitiAgent.Stop() is handled by defer above

	// Stop all traders
	traderManager.StopAll()
	logger.Info("✅ System shut down safely")
}

// newSharedMCPClient creates a shared MCP AI client (for backtesting)
func newSharedMCPClient() mcp.AIClient {
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		logger.Warn("⚠️ DEEPSEEK_API_KEY not set, AI backtest features will be unavailable")
		return nil
	}
	return mcp.NewAIClientByProvider("deepseek")
}
