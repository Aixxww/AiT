# Changelog

All notable changes to the AiT project will be documented in this file.

> **Note:** AiT was formerly known as NOFX. Entries before v4.0.0 are preserved under their original naming.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

**Languages:** [English](CHANGELOG.md) | [中文](CHANGELOG.zh-CN.md)

---

## [Unreleased]

### Added
- Hunter bidirectional coin selection: simultaneous LONG and SHORT signal output
- Hunter R7 live analysis report (2026-05-23) with BOTH direction scoring
- Hunter evaluation report for UNIUSDT (2026-05-23)
- Documentation system with multi-language support (EN/CN/RU/UK/JP/VN/KR)
- Complete getting-started guides (Docker, Custom API, Binance, OKX, Bybit, Hyperliquid, Aster, Lighter)
- Architecture documentation with system design details
- User guides with FAQ and troubleshooting
- Community documentation with bounty programs
- Hunter module docs section in docs center

### Changed
- Reorganized documentation structure into logical categories
- Updated all README files with proper navigation links
- Replaced all NOFX branding with AiT across documentation (50+ files)
- Updated GitHub repository URLs to Aixxww/AiT
- Kernel engine optimizations (engine.go, engine_analysis.go, engine_position.go, engine_prompt.go)
- Market data layer improvements (data.go, data_klines.go)
- Provider module optimizations (local/client.go, local/hunter.go, nofxos/ai500.go)

---

## [4.0.0] - 2026-05-20

### Added — AiT Platform: Multi-AI, Multi-Exchange, Social Trading

**Complete rebrand from NOFX to AiT with major feature additions.**

#### Brand & Identity
- Renamed project from NOFX to AiT across all code, configs, and UI
- New logo and branding assets
- Removed nofxos.ai dependency

#### Hunter Coin Selection Module (`provider/hunter/`)
- Smart money flow detection with multi-timeframe analysis
- CoinGecko fallback when Binance API is unavailable
- Cooldown persistence across restarts
- Open interest deduplication logic
- Backtest & optimization toolkit (`Hunter Validator`)

#### AI500 Scoring Engine (`provider/local/`)
- Momentum-based coin ranking across 500+ pairs
- Scoring algorithm emphasises volatility over raw volume
- CoinAnk data fallback with funding rate integration
- Parallelised data fetching for faster response

#### Binance Klines Direct Integration (`market/data_klines.go`)
- Direct Binance public API for real-time candlestick data
- Dual-source strategy: Binance primary → CoinAnk fallback
- No trailing empty candles, real-time volume data

#### Social Sentiment (`provider/square/` + `scripts/square-monitor/`)
- Square Monitor Python service with Playwright scraping
- 6-minute refresh interval for heat signals
- Composite score filtering for trade signal generation

#### Full-Chain Token Usage Tracking
- Token consumption tracking across all AI providers
- Anti-repeat filter to avoid duplicate AI calls
- Frontend usage display per trader

#### One-Click Install & Start Scripts
- `scripts/install.sh` — auto-detect OS, install Go/Node/TA-Lib
- `scripts/start.sh` — background start backend + frontend + square-monitor
- Docker mode and minimal install options

#### Frontend Strategy Studio Enhancements
- 8-parameter coin selection panel with real-time tuning
- Hunter/AI500 source type support in strategy editor
- CoinSourceEditor component for coin source configuration
- i18n support for new strategy types

#### Exchange Improvements
- Improved futures position tracking (Binance)
- Enhanced order sync logic (Binance)
- Auto-trader refinement

### Changed
- MiMo AI model timeout extended to 8 minutes
- Square Heat refresh interval reduced to 6 minutes
- Go dependency cleanup and module optimization

---

## [3.0.0] - 2025-10-30

### Added - Major Architecture Transformation 🚀

**Complete System Redesign - Web-Based Configuration Platform**

This is a **major breaking update** that completely transforms NOFX from a static config-based system to a modern web-based trading platform.

#### Database-Driven Architecture
- SQLite integration replacing static JSON config
- Persistent storage with automatic timestamps
- Foreign key relationships and triggers for data consistency
- Separate tables for AI models, exchanges, traders, and system config

#### Web-Based Configuration Interface
- Complete web-based configuration management (no more JSON editing)
- AI Model setup through web interface (DeepSeek/Qwen API keys)
- Exchange management (Binance/Hyperliquid credentials)
- Dynamic trader creation (combine any AI model with any exchange)
- Real-time control (start/stop traders without system restart)

#### Flexible Architecture
- Separation of concerns (AI models and exchanges independent)
- Mix & match capability (unlimited combinations)
- Scalable design (support for unlimited traders)
- Clean slate approach (no default traders)

#### Enhanced API Layer
- RESTful design with complete CRUD operations
- New endpoints:
  - `GET/PUT /api/models` - AI model configuration
  - `GET/PUT /api/exchanges` - Exchange configuration
  - `POST/DELETE /api/traders` - Trader management
  - `POST /api/traders/:id/start|stop` - Trader control
- Updated documentation for all API endpoints

#### Modernized Codebase
- Type safety with proper separation of configuration types
- Database abstraction with prepared statements
- Comprehensive error handling and validation
- Better code organization (database, API, business logic)

### Changed
- **BREAKING**: Old `config.json` files no longer used
- Configuration must be done through web interface
- Much easier setup and better UX
- No more server restarts for configuration changes

### Why This Matters
- 🎯 **User Experience**: Much easier to configure and manage
- 🔧 **Flexibility**: Create any combination of AI models and exchanges
- 📊 **Scalability**: Support for complex multi-trader setups
- 🔒 **Reliability**: Database ensures data persistence and consistency
- 🚀 **Future-Proof**: Foundation for advanced features

---

## [2.0.2] - 2025-10-29

### Fixed - Critical Bug Fixes: Trade History & Performance Analysis

#### PnL Calculation - Major Error Fixed
- **Fixed**: PnL now calculated as actual USDT amount instead of percentage only
- Previously ignored position size and leverage (e.g., 100 USDT @ 5% = 1000 USDT @ 5%)
- Now: `PnL (USDT) = Position Value × Price Change % × Leverage`
- Impact: Win rate, profit factor, and Sharpe ratio now accurate

#### Position Tracking - Missing Critical Data
- **Fixed**: Open position records now store quantity and leverage
- Previously only stored price and time
- Essential for accurate PnL calculations

#### Position Key Logic - Long/Short Conflict
- **Fixed**: Changed from `symbol` to `symbol_side` format
- Now properly distinguishes between long and short positions
- Example: `BTCUSDT_long` vs `BTCUSDT_short`

#### Sharpe Ratio Calculation - Code Optimization
- **Changed**: Replaced custom Newton's method with `math.Sqrt`
- More reliable, maintainable, and efficient

### Why This Matters
- Historical trade statistics now show real USDT profit/loss
- Performance comparison between different leverage trades is accurate
- AI self-learning mechanism receives correct feedback
- Multi-position tracking (long + short simultaneously) works correctly

---

## [2.0.2] - 2025-10-29

### Fixed - Aster Exchange Precision Error

- Fixed Aster exchange precision error (code -1111)
- Improved price and quantity formatting to match exchange requirements
- Added detailed precision processing logs for debugging
- Enhanced all order functions with proper precision handling

#### Technical Details
- Added `formatFloatWithPrecision` function
- Price and quantity formatted according to exchange specifications
- Trailing zeros removed to optimize API requests

---

## [2.0.1] - 2025-10-29

### Fixed - ComparisonChart Data Processing

- Fixed ComparisonChart data processing logic
- Switched from cycle_number to timestamp grouping
- Resolved chart freezing issue when backend restarts
- Improved chart data display (shows all historical data chronologically)
- Enhanced debugging logs

---

## [2.0.0] - 2025-10-28

### Added - Major Updates

- AI self-learning mechanism (historical feedback, performance analysis)
- Multi-trader competition mode (Qwen vs DeepSeek)
- Binance-style UI (complete interface imitation)
- Performance comparison charts (real-time ROI comparison)
- Risk control optimization (per-coin position limit adjustment)

### Fixed

- Fixed hardcoded initial balance issue
- Fixed multi-trader data sync issue
- Optimized chart data alignment (using cycle_number)

---

## [1.0.0] - 2025-10-27

### Added - Initial Release

- Basic AI trading functionality
- Decision logging system
- Simple Web interface
- Support for Binance Futures
- DeepSeek and Qwen AI model integration

---

## How to Use This Changelog

### For Users
- Check the [Unreleased] section for upcoming features
- Review version sections to understand what changed
- Follow migration guides for breaking changes

### For Contributors
When making changes, add them to the [Unreleased] section under appropriate categories:
- **Added** - New features
- **Changed** - Changes to existing functionality
- **Deprecated** - Features that will be removed
- **Removed** - Features that were removed
- **Fixed** - Bug fixes
- **Security** - Security fixes

When releasing a new version, move [Unreleased] items to a new version section with date.

---

## Links

- [Documentation](docs/README.md)
- [Contributing Guidelines](CONTRIBUTING.md)
- [Security Policy](SECURITY.md)
- [GitHub Repository](https://github.com/Aixxww/AiT)

---

**Last Updated:** 2026-05-20
