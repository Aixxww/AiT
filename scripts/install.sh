#!/bin/bash
#
# AiT — One-Click Installation Script
# https://github.com/Aixxww/AiT
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/Aixxww/AiT/main/scripts/install.sh | bash
#
# Options:
#   --dev        Development mode (default) — install all deps, build from source
#   --docker     Docker mode — install Docker, pull images
#   --minimal    Skip Python / Square Monitor
#   --dir DIR    Installation directory (default: current directory or ~/AiT)
#   --backend-port PORT   Backend API port (default: 8080)
#   --frontend-port PORT  Frontend dashboard port (default: 3000)
#   --proxy URL           HTTP/HTTPS/SOCKS5 proxy for dependency + Binance access
#   --timezone TZ         Runtime timezone (default: Asia/Shanghai)
#   --menu                Open the interactive AiT management menu after install
#

set -euo pipefail

# ─── Colors ────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

# ─── Globals ───────────────────────────────────────────────────
MODE="dev"
INSTALL_DIR=""
GO_VERSION="1.25.3"
NODE_MAJOR=20
PYTHON_MIN="3.10"
MINIMAL=0
BACKEND_PORT="${AIT_BACKEND_PORT:-8080}"
FRONTEND_PORT="${AIT_FRONTEND_PORT:-3000}"
TIMEZONE="${AIT_TIMEZONE:-Asia/Shanghai}"
PROXY_URL="${AIT_PROXY_URL:-${AIT_BINANCE_PROXY_URL:-}}"
OPEN_MENU=0

# ─── Helpers ───────────────────────────────────────────────────
info()    { echo -e "${BLUE}[INFO]${NC} $*"; }
ok()      { echo -e "${GREEN}[OK]${NC} $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC} $*"; }
err()     { echo -e "${RED}[ERROR]${NC} $*"; }
step()    { echo -e "\n${CYAN}${BOLD}▶ $*${NC}"; }
divider() { echo -e "${CYAN}──────────────────────────────────────────────────${NC}"; }

die() { err "$*"; exit 1; }

# ─── Parse Arguments ──────────────────────────────────────────
while [[ $# -gt 0 ]]; do
    case $1 in
        --dev)      MODE="dev"; shift ;;
        --docker)   MODE="docker"; shift ;;
        --minimal)  MINIMAL=1; shift ;;
        --dir)      INSTALL_DIR="$2"; shift 2 ;;
        --backend-port)  BACKEND_PORT="$2"; shift 2 ;;
        --frontend-port) FRONTEND_PORT="$2"; shift 2 ;;
        --proxy)     PROXY_URL="$2"; shift 2 ;;
        --timezone)  TIMEZONE="$2"; shift 2 ;;
        --menu)      OPEN_MENU=1; shift ;;
        -h|--help)
            echo "AiT One-Click Installer"
            echo ""
            echo "Usage: install.sh [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --dev       Development mode (default)"
            echo "  --docker    Docker mode"
            echo "  --minimal   Skip Python / Square Monitor"
            echo "  --dir DIR   Installation directory"
            echo "  --backend-port PORT   Backend API port (default: 8080)"
            echo "  --frontend-port PORT  Frontend dashboard port (default: 3000)"
            echo "  --proxy URL           Proxy URL, e.g. http://127.0.0.1:7897"
            echo "  --timezone TZ         Runtime timezone (default: Asia/Shanghai)"
            echo "  --menu                Open interactive management menu after install"
            echo "  -h, --help  Show this help"
            exit 0
            ;;
        *) die "Unknown option: $1" ;;
    esac
done

validate_port() {
    local name="$1" value="$2"
    [[ "$value" =~ ^[0-9]+$ ]] || die "$name must be a number: $value"
    (( value >= 1 && value <= 65535 )) || die "$name must be between 1 and 65535: $value"
}

validate_config() {
    validate_port "backend port" "$BACKEND_PORT"
    validate_port "frontend port" "$FRONTEND_PORT"
    [[ "$BACKEND_PORT" != "$FRONTEND_PORT" ]] || die "backend and frontend ports must be different"
}

# ─── OS Detection ─────────────────────────────────────────────
detect_os() {
    step "Detecting system environment"

    OS="$(uname -s)"
    ARCH="$(uname -m)"

    case "$OS" in
        Darwin)
            OS_NAME="macos"
            if [[ "$ARCH" == "arm64" ]]; then
                ARCH_NAME="arm64"
                info "macOS Apple Silicon (arm64)"
            else
                ARCH_NAME="amd64"
                info "macOS Intel (amd64)"
            fi
            ;;
        Linux)
            if [[ -f /etc/os-release ]]; then
                . /etc/os-release
                case "$ID" in
                    ubuntu|debian|linuxmint|pop)    OS_NAME="debian" ;;
                    centos|rhel|rocky|alma|fedora)  OS_NAME="rhel" ;;
                    arch|manjaro|endeavouros)       OS_NAME="arch" ;;
                    alpine)                         OS_NAME="alpine" ;;
                    *)                              OS_NAME="linux" ;;
                esac
                info "Linux $PRETTY_NAME ($ARCH)"
            else
                OS_NAME="linux"
                info "Linux ($ARCH)"
            fi
            case "$ARCH" in
                x86_64)  ARCH_NAME="amd64" ;;
                aarch64) ARCH_NAME="arm64" ;;
                *)       ARCH_NAME="$ARCH" ;;
            esac
            ;;
        MINGW*|MSYS*|CYGWIN*)
            OS_NAME="windows"
            case "$ARCH" in
                x86_64|AMD64) ARCH_NAME="amd64" ;;
                aarch64|ARM64) ARCH_NAME="arm64" ;;
                *) ARCH_NAME="$ARCH" ;;
            esac
            warn "Windows shell detected. Native dev mode is not supported by this Bash installer."
            warn "Use --docker with Docker Desktop, or run this script inside WSL2 for dev mode."
            ;;
        *)
            die "Unsupported OS: $OS"
            ;;
    esac

    ok "Detected: $OS_NAME / $ARCH_NAME"
}

configure_proxy() {
    if [[ -z "$PROXY_URL" ]]; then
        info "Proxy: not configured"
        return
    fi

    step "Configuring proxy"
    info "Proxy URL: $PROXY_URL"
    info "This proxy is exported for curl/npm/go/docker pull where supported, and persisted for AiT Binance HTTP clients."
    export HTTP_PROXY="$PROXY_URL"
    export HTTPS_PROXY="$PROXY_URL"
    export ALL_PROXY="$PROXY_URL"
    export http_proxy="$PROXY_URL"
    export https_proxy="$PROXY_URL"
    export all_proxy="$PROXY_URL"
    export AIT_BINANCE_PROXY_URL="$PROXY_URL"
    export NO_PROXY="${NO_PROXY:-localhost,127.0.0.1,::1}"
    export no_proxy="${no_proxy:-localhost,127.0.0.1,::1}"
    ok "Proxy variables exported for this install run"
}

# ─── Command Existence ────────────────────────────────────────
has() { command -v "$1" &>/dev/null; }

# ─── Version Comparison ───────────────────────────────────────
# Returns 0 if $1 >= $2
version_gte() {
    printf '%s\n%s' "$2" "$1" | sort -V -C
}

# ─── Install Go ───────────────────────────────────────────────
install_go() {
    step "Checking Go"

    if [[ "${OS_NAME:-}" == "windows" ]]; then
        die "Windows dev mode is not supported here. Use --docker with Docker Desktop or run inside WSL2."
    fi

    if has go; then
        local current
        current=$(go version | grep -oE '[0-9]+\.[0-9]+(\.[0-9]+)?' | head -1)
        if version_gte "$current" "1.25"; then
            ok "Go $current already installed"
            return
        fi
        warn "Go $current is too old, need 1.25+"
    fi

    info "Installing Go $GO_VERSION..."

    local go_tar="go${GO_VERSION}.${OS_NAME/linux/linux}-${ARCH_NAME}.tar.gz"
    local go_url="https://go.dev/dl/${go_tar}"

    case "$OS_NAME" in
        macos)
            if has brew; then
                brew install go@1.25 2>/dev/null || brew upgrade go@1.25 2>/dev/null || true
                ok "Go installed via Homebrew"
                return
            fi
            ;;
    esac

    # Universal: download tarball
    local tmp_dir
    tmp_dir=$(mktemp -d)
    info "Downloading $go_url ..."
    curl -fsSL "$go_url" -o "$tmp_dir/$go_tar"

    # Remove old Go if exists
    sudo rm -rf /usr/local/go 2>/dev/null || true

    info "Extracting to /usr/local/go ..."
    sudo tar -C /usr/local -xzf "$tmp_dir/$go_tar"
    rm -rf "$tmp_dir"

    # Add to PATH
    export PATH="/usr/local/go/bin:$HOME/go/bin:$PATH"

    # Persist in shell profile
    local profile=""
    if [[ -f "$HOME/.zshrc" ]]; then
        profile="$HOME/.zshrc"
    elif [[ -f "$HOME/.bashrc" ]]; then
        profile="$HOME/.bashrc"
    elif [[ -f "$HOME/.bash_profile" ]]; then
        profile="$HOME/.bash_profile"
    fi

    if [[ -n "$profile" ]]; then
        if ! grep -q '/usr/local/go/bin' "$profile" 2>/dev/null; then
            echo 'export PATH="/usr/local/go/bin:$HOME/go/bin:$PATH"' >> "$profile"
            info "Added Go to PATH in $profile"
        fi
    fi

    ok "Go $GO_VERSION installed"
}

# ─── Install Node.js ──────────────────────────────────────────
install_node() {
    step "Checking Node.js"

    if [[ "${OS_NAME:-}" == "windows" ]]; then
        die "Windows dev mode is not supported here. Use --docker with Docker Desktop or run inside WSL2."
    fi

    if has node; then
        local current
        current=$(node -v | tr -d 'v')
        if version_gte "$(echo "$current" | cut -d. -f1)" "$NODE_MAJOR"; then
            ok "Node.js v$current already installed"
            return
        fi
        warn "Node.js v$current is too old, need v${NODE_MAJOR}+"
    fi

    info "Installing Node.js v${NODE_MAJOR}..."

    case "$OS_NAME" in
        macos)
            if has brew; then
                brew install node@20 2>/dev/null || brew upgrade node@20 2>/dev/null || true
                ok "Node.js installed via Homebrew"
                return
            fi
            ;;
        debian)
            curl -fsSL "https://deb.nodesource.com/setup_${NODE_MAJOR}.x" | sudo -E bash -
            sudo apt-get install -y nodejs
            ok "Node.js installed via NodeSource"
            return
            ;;
        rhel)
            curl -fsSL "https://rpm.nodesource.com/setup_${NODE_MAJOR}.x" | sudo bash -
            sudo yum install -y nodejs
            ok "Node.js installed via NodeSource"
            return
            ;;
        arch)
            sudo pacman -S --noconfirm nodejs npm
            ok "Node.js installed via pacman"
            return
            ;;
    esac

    # Fallback: fnm or nvm
    if has fnm; then
        fnm install $NODE_MAJOR
        fnm use $NODE_MAJOR
        ok "Node.js installed via fnm"
    elif has nvm; then
        nvm install $NODE_MAJOR
        nvm use $NODE_MAJOR
        ok "Node.js installed via nvm"
    else
        die "Cannot install Node.js automatically. Please install Node.js ${NODE_MAJOR}+ manually: https://nodejs.org/"
    fi
}

# ─── Install TA-Lib (C library) ───────────────────────────────
install_talib() {
    step "Checking TA-Lib"

    if [[ "${OS_NAME:-}" == "windows" ]]; then
        die "Windows dev mode is not supported here because TA-Lib C dependencies are not installed natively. Use --docker or WSL2."
    fi

    # Check if already installed
    if pkg-config --exists ta-lib 2>/dev/null; then
        ok "TA-Lib already installed"
        return
    fi

    if [[ -f /usr/local/lib/libta_lib.a ]] || [[ -f /usr/lib/libta_lib.a ]] || [[ -f /opt/homebrew/lib/libta_lib.a ]]; then
        ok "TA-Lib already installed"
        return
    fi

    info "Installing TA-Lib (C library)..."

    case "$OS_NAME" in
        macos)
            if has brew; then
                brew install ta-lib
                ok "TA-Lib installed via Homebrew"
                return
            fi
            ;;
        debian)
            sudo apt-get update -qq
            sudo apt-get install -y libta-lib0-dev || {
                # Some distros don't have it, build from source
                warn "Package not found, building from source..."
                _build_talib_from_source
            }
            ok "TA-Lib installed"
            return
            ;;
        rhel)
            sudo yum install -y ta-lib-devel 2>/dev/null || {
                warn "Package not found, building from source..."
                _build_talib_from_source
            }
            ok "TA-Lib installed"
            return
            ;;
        arch)
            # Try AUR helper
            if has yay; then
                yay -S --noconfirm ta-lib
            elif has paru; then
                paru -S --noconfirm ta-lib
            else
                _build_talib_from_source
            fi
            ok "TA-Lib installed"
            return
            ;;
    esac

    _build_talib_from_source
    ok "TA-Lib installed from source"
}

_build_talib_from_source() {
    local tmp_dir
    tmp_dir=$(mktemp -d)

    # Install build deps
    case "$OS_NAME" in
        debian) sudo apt-get install -y build-essential wget ;;
        rhel)   sudo yum groupinstall -y "Development Tools" && sudo yum install -y wget ;;
        arch)   sudo pacman -S --noconfirm base-devel wget ;;
    esac

    info "Downloading TA-Lib source..."
    curl -fsSL "https://github.com/TA-Lib/ta-lib/releases/download/v0.4.0/ta-lib-0.4.0-src.tar.gz" \
        -o "$tmp_dir/ta-lib.tar.gz"

    cd "$tmp_dir"
    tar xzf ta-lib.tar.gz
    cd ta-lib/

    info "Compiling TA-Lib..."
    ./configure --prefix=/usr/local
    make -j"$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 2)"
    sudo make install
    sudo ldconfig 2>/dev/null || true

    cd /
    rm -rf "$tmp_dir"
}

# ─── Install Python (optional) ────────────────────────────────
install_python() {
    if [[ "${MINIMAL:-0}" == "1" ]]; then
        info "Skipping Python (--minimal mode)"
        return
    fi

    step "Checking Python"

    local py_cmd=""
    if has python3; then
        py_cmd="python3"
    elif has python; then
        py_cmd="python"
    fi

    if [[ -n "$py_cmd" ]]; then
        local ver
        ver=$($py_cmd --version 2>&1 | grep -oE '[0-9]+\.[0-9]+')
        if version_gte "$ver" "$PYTHON_MIN"; then
            ok "Python $ver already installed"
            return
        fi
        warn "Python $ver is too old, need ${PYTHON_MIN}+"
    fi

    info "Python ${PYTHON_MIN}+ is needed for Square Monitor (optional)"
    info "Install manually: https://www.python.org/downloads/"
    warn "Square Monitor will be unavailable until Python is installed"
}

# ─── Install Docker ───────────────────────────────────────────
install_docker() {
    step "Checking Docker"

    if has docker && docker info &>/dev/null; then
        ok "Docker is installed and running"
        return
    fi

    info "Installing Docker..."

    case "$OS_NAME" in
        windows)
            if has docker; then
                die "Docker CLI exists but Docker Desktop is not responding. Start Docker Desktop, then rerun: scripts/install.sh --docker"
            fi
            die "Install Docker Desktop for Windows first: https://docs.docker.com/desktop/setup/install/windows-install/"
            ;;
        macos)
            if has brew; then
                brew install --cask docker
                ok "Docker Desktop installed (please open it once)"
                return
            fi
            die "Please install Docker Desktop: https://docs.docker.com/get-docker/"
            ;;
        debian)
            # Official Docker install script
            curl -fsSL https://get.docker.com | sudo sh
            sudo usermod -aG docker "$USER"
            ok "Docker installed (log out and back in for group changes)"
            return
            ;;
        rhel)
            curl -fsSL https://get.docker.com | sudo sh
            sudo systemctl start docker
            sudo systemctl enable docker
            sudo usermod -aG docker "$USER"
            ok "Docker installed and started"
            return
            ;;
        arch)
            sudo pacman -S --noconfirm docker docker-compose
            sudo systemctl start docker
            sudo systemctl enable docker
            sudo usermod -aG docker "$USER"
            ok "Docker installed and started"
            return
            ;;
    esac

    die "Please install Docker manually: https://docs.docker.com/get-docker/"
}

# ─── Setup Project ────────────────────────────────────────────
setup_project() {
    step "Setting up AiT project"

    # Determine install directory
    if [[ -z "$INSTALL_DIR" ]]; then
        # If we're already inside an AiT directory, use current dir
        if [[ -f "main.go" && -f "go.mod" ]]; then
            INSTALL_DIR="$(pwd)"
            info "Using current directory: $INSTALL_DIR"
        else
            INSTALL_DIR="$HOME/AiT"
        fi
    fi

    # Clone or verify
    if [[ -d "$INSTALL_DIR/.git" ]]; then
        info "Repository exists at $INSTALL_DIR"
        cd "$INSTALL_DIR"
        info "Pulling latest..."
        git pull --ff-only 2>/dev/null || warn "Could not pull (local changes?)"
    elif [[ -f "$INSTALL_DIR/main.go" ]]; then
        cd "$INSTALL_DIR"
        info "Using existing project at $INSTALL_DIR"
    else
        info "Cloning AiT to $INSTALL_DIR..."
        git clone https://github.com/Aixxww/AiT.git "$INSTALL_DIR"
        cd "$INSTALL_DIR"
    fi

    ok "Project directory: $INSTALL_DIR"
}

# ─── Generate .env ────────────────────────────────────────────
generate_env() {
    step "Configuring environment"

    if [[ -f ".env" ]]; then
        ok ".env already exists, preserving secrets and updating runtime ports/proxy"
        _set_env "AIT_BACKEND_PORT" "$BACKEND_PORT"
        _set_env "AIT_FRONTEND_PORT" "$FRONTEND_PORT"
        _set_env "AIT_TIMEZONE" "$TIMEZONE"
        if [[ -n "$PROXY_URL" ]]; then
            _set_env "AIT_PROXY_URL" "$PROXY_URL"
            _set_env "AIT_BINANCE_PROXY_URL" "$PROXY_URL"
            _set_env "HTTP_PROXY" "$PROXY_URL"
            _set_env "HTTPS_PROXY" "$PROXY_URL"
            _set_env "ALL_PROXY" "$PROXY_URL"
            _set_env "NO_PROXY" "localhost,127.0.0.1,::1"
        fi
        return
    fi

    info "Generating encryption keys..."

    local jwt_secret data_key rsa_key

    if has openssl; then
        jwt_secret=$(openssl rand -base64 32)
        data_key=$(openssl rand -base64 32)
        rsa_key=$(openssl genrsa 2048 2>/dev/null)
    else
        # Fallback: use Go
        jwt_secret=$(head -c 32 /dev/urandom | base64)
        data_key=$(head -c 32 /dev/urandom | base64)
        rsa_key=""
        warn "openssl not found — RSA key generation skipped (will be auto-generated on first run)"
    fi

    # Format RSA key for single line
    local rsa_escaped=""
    if [[ -n "$rsa_key" ]]; then
        rsa_escaped=$(echo "$rsa_key" | awk '{printf "%s\\n", $0}')
        rsa_escaped="\"${rsa_escaped}\""
    fi

    cat > .env << EOF
# AiT Configuration
# Generated: $(date -u +"%Y-%m-%dT%H:%M:%SZ")

# ── Server ──────────────────────────────────────────
AIT_BACKEND_PORT=${BACKEND_PORT}
AIT_FRONTEND_PORT=${FRONTEND_PORT}
AIT_TIMEZONE=${TIMEZONE}

# ── Network Proxy ──────────────────────────────────
# Set when your server needs a proxy for model/exchange/dependency access.
# Example: http://127.0.0.1:7897 or socks5://127.0.0.1:1080
AIT_PROXY_URL=${PROXY_URL}
AIT_BINANCE_PROXY_URL=${PROXY_URL}
HTTP_PROXY=${PROXY_URL}
HTTPS_PROXY=${PROXY_URL}
ALL_PROXY=${PROXY_URL}
NO_PROXY=localhost,127.0.0.1,::1

# ── Authentication ──────────────────────────────────
JWT_SECRET=${jwt_secret}

# ── Encryption ──────────────────────────────────────
# AES-256 key (Base64, 32 bytes) — encrypts API keys in database
DATA_ENCRYPTION_KEY=${data_key}

# RSA 2048-bit PEM — client-server key exchange
# Auto-generated on first run if empty
RSA_PRIVATE_KEY=${rsa_escaped}

# ── Security ────────────────────────────────────────
# Browser-side API key encryption (requires HTTPS or localhost)
TRANSPORT_ENCRYPTION=false

# ── Database ────────────────────────────────────────
DB_TYPE=sqlite
DB_PATH=data/data.db
# For PostgreSQL:
# DB_TYPE=postgres
# DB_HOST=localhost
# DB_PORT=5432
# DB_USER=ait
# DB_PASSWORD=
# DB_NAME=ait
# DB_SSLMODE=disable
EOF

    chmod 600 .env 2>/dev/null || true
    mkdir -p data
    ok ".env created with auto-generated keys"
    warn "Keep .env safe — do NOT commit to version control"
}

# ─── Helper: set env var ──────────────────────────────────────
_set_env() {
    local name="$1" value="$2" file=".env"
    if grep -q "^${name}=" "$file" 2>/dev/null; then
        if [[ "$OSTYPE" == "darwin"* ]]; then
            sed -i '' "s|^${name}=.*|${name}=${value}|" "$file"
        else
            sed -i "s|^${name}=.*|${name}=${value}|" "$file"
        fi
    else
        echo "${name}=${value}" >> "$file"
    fi
}

# ─── Install Dependencies ─────────────────────────────────────
install_deps() {
    step "Installing project dependencies"

    # Go deps
    info "Downloading Go modules..."
    go mod download
    ok "Go dependencies ready"

    # Frontend deps
    info "Installing frontend packages..."
    cd web
    npm install --no-fund --no-audit
    cd ..
    ok "Frontend dependencies ready"

    # Python deps (optional)
    if [[ "${MINIMAL:-0}" != "1" ]] && [[ -d "scripts/square-monitor" ]]; then
        if has python3 || has python; then
            info "Setting up Square Monitor (Python)..."
            cd scripts/square-monitor

            if [[ ! -d ".venv" ]]; then
                python3 -m venv .venv 2>/dev/null || python -m venv .venv 2>/dev/null || {
                    warn "Could not create Python venv"
                    cd ../..
                    return
                }
            fi

            source .venv/bin/activate
            pip install -q -r requirements.txt 2>/dev/null || warn "pip install had issues"
            deactivate
            cd ../..
            ok "Square Monitor dependencies ready"
        else
            info "Skipping Square Monitor (no Python found)"
        fi
    fi
}

# ─── Build ────────────────────────────────────────────────────
build_project() {
    step "Building AiT"

    info "Building Go backend..."
    CGO_ENABLED=1 go build -o ait .
    ok "Backend built: ./ait"

    info "Building frontend..."
    cd web
    npm run build 2>&1 | tail -1
    cd ..
    ok "Frontend built: ./web/dist"
}

# ─── Docker Mode ──────────────────────────────────────────────
docker_mode() {
    validate_config
    detect_os
    configure_proxy
    install_docker
    setup_project
    generate_env

    step "Starting Docker services"
    info "Docker mode uses docker-compose.yml/docker-compose.prod.yml port mappings from .env:"
    info "  AIT_BACKEND_PORT=$BACKEND_PORT -> container :8080"
    info "  AIT_FRONTEND_PORT=$FRONTEND_PORT -> container :80"
    [[ -n "$PROXY_URL" ]] && info "  Proxy persisted: $PROXY_URL"

    local compose_cmd="docker compose"
    if ! docker compose version &>/dev/null; then
        if has docker-compose; then
            compose_cmd="docker-compose"
        else
            die "Docker Compose not found"
        fi
    fi

    local compose_file="${AIT_COMPOSE_FILE:-}"
    if [[ -z "$compose_file" ]]; then
        if [[ -f "docker-compose.prod.yml" ]]; then
            compose_file="docker-compose.prod.yml"
        else
            compose_file="docker-compose.yml"
        fi
    fi
    info "Docker Compose file: $compose_file"

    $compose_cmd -f "$compose_file" pull
    $compose_cmd -f "$compose_file" up -d

    echo ""
    ok "Docker services started!"
    echo ""
    echo -e "  ${BOLD}Web Dashboard:${NC}  http://localhost:$FRONTEND_PORT"
    echo -e "  ${BOLD}API Endpoint:${NC}   http://localhost:$BACKEND_PORT"
    echo ""
    echo -e "  ${YELLOW}Manage:${NC}  $compose_cmd -f $compose_file logs -f"
    echo -e "  ${YELLOW}Stop:${NC}    $compose_cmd -f $compose_file down"
    echo ""
}

# ─── Dev Mode ─────────────────────────────────────────────────
dev_mode() {
    validate_config
    detect_os
    configure_proxy
    install_go
    install_node
    install_talib
    install_python
    setup_project
    generate_env
    install_deps
    build_project

    divider
    echo ""
    echo -e "${GREEN}${BOLD}  AiT installation complete!${NC}"
    echo ""
    echo -e "  ${BOLD}Quick start:${NC}"
    echo -e "    cd $INSTALL_DIR"
    echo -e "    ./scripts/start.sh dev"
    echo ""
    echo -e "  ${BOLD}Web Dashboard:${NC}  http://localhost:$FRONTEND_PORT"
    echo -e "  ${BOLD}API Endpoint:${NC}   http://localhost:$BACKEND_PORT"
    [[ -n "$PROXY_URL" ]] && echo -e "  ${BOLD}Proxy:${NC}          $PROXY_URL"
    echo ""
    echo -e "  ${BOLD}Next steps:${NC}"
    echo "    1. Open http://localhost:3000 and register"
    echo "    2. Add an AI model (Settings -> AI Models)"
    echo "    3. Add an exchange (Settings -> Exchanges)"
    echo "    4. Create a strategy in Strategy Studio"
    echo "    5. Create a trader and start!"
    echo ""
    echo -e "  ${YELLOW}Docs:${NC}  https://github.com/Aixxww/AiT"
    echo ""
    echo -e "  ${RED}AI trading carries significant risk. Only use funds you can afford to lose.${NC}"
    echo ""

    if [[ "$OPEN_MENU" == "1" && -x "$INSTALL_DIR/scripts/start.sh" ]]; then
        "$INSTALL_DIR/scripts/start.sh" menu
    fi
}

# ─── Main ─────────────────────────────────────────────────────
main() {
    echo -e "${CYAN}"
    echo "  ╔═══════════════════════════════════════════════════╗"
    echo "  ║         AiT — AI Trading System Installer          ║"
    echo "  ╚═══════════════════════════════════════════════════╝"
    echo -e "${NC}"

    case "$MODE" in
        docker) docker_mode ;;
        *)      dev_mode ;;
    esac
}

main
