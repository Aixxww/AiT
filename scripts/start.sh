#!/bin/bash
#
# AiT — Unified Startup Script
# https://github.com/Aixxww/AiT
#
# Usage:
#   ./scripts/start.sh [mode] [command]
#
# Modes:
#   dev    Development mode (default) — run backend + frontend + optional square-monitor
#   docker Docker mode — manage Docker Compose services
#   prod   Production mode — build everything, run as background processes
#
# Commands (docker mode only):
#   start, stop, restart, logs, status, clean, update, regenerate-keys
# Interactive:
#   ./scripts/start.sh menu
#

set -euo pipefail

# ─── Constants ─────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
PID_DIR="$PROJECT_DIR/.pids"
LOG_DIR="$PROJECT_DIR/.logs"

# ─── Colors ────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m'

# ─── Helpers ───────────────────────────────────────────────────
info()    { echo -e "${BLUE}[$(date +%H:%M:%S)]${NC} $*"; }
ok()      { echo -e "${GREEN}[$(date +%H:%M:%S)] ✓${NC} $*"; }
warn()    { echo -e "${YELLOW}[$(date +%H:%M:%S)] !${NC} $*"; }
err()     { echo -e "${RED}[$(date +%H:%M:%S)] ✗${NC} $*" >&2; }
step()    { echo -e "\n${CYAN}${BOLD}▶ $*${NC}"; }
divider() { echo -e "${DIM}────────────────────────────────────────────────────${NC}"; }
has()     { command -v "$1" &>/dev/null; }
die()     { err "$*"; exit 1; }

# ─── Load .env ─────────────────────────────────────────────────
load_env() {
    if [[ -f "$PROJECT_DIR/.env" ]]; then
        set -a
        # shellcheck disable=SC1091
        source "$PROJECT_DIR/.env"
        set +a
    fi
    refresh_runtime_config
}

# ─── Port config ───────────────────────────────────────────────
BACKEND_PORT="${AIT_BACKEND_PORT:-8080}"
FRONTEND_PORT="${AIT_FRONTEND_PORT:-3000}"
TIMEZONE="${AIT_TIMEZONE:-Asia/Shanghai}"
PROXY_URL="${AIT_PROXY_URL:-${AIT_BINANCE_PROXY_URL:-}}"

refresh_runtime_config() {
    BACKEND_PORT="${AIT_BACKEND_PORT:-8080}"
    FRONTEND_PORT="${AIT_FRONTEND_PORT:-3000}"
    TIMEZONE="${AIT_TIMEZONE:-Asia/Shanghai}"
    PROXY_URL="${AIT_PROXY_URL:-${AIT_BINANCE_PROXY_URL:-}}"
}

export_runtime_env() {
    export API_SERVER_PORT="$BACKEND_PORT"
    export TZ="$TIMEZONE"
    export AIT_BINANCE_PROXY_URL="${AIT_BINANCE_PROXY_URL:-$PROXY_URL}"
    if [[ -n "$PROXY_URL" ]]; then
        export HTTP_PROXY="${HTTP_PROXY:-$PROXY_URL}"
        export HTTPS_PROXY="${HTTPS_PROXY:-$PROXY_URL}"
        export ALL_PROXY="${ALL_PROXY:-$PROXY_URL}"
        export http_proxy="${http_proxy:-$PROXY_URL}"
        export https_proxy="${https_proxy:-$PROXY_URL}"
        export all_proxy="${all_proxy:-$PROXY_URL}"
        export NO_PROXY="${NO_PROXY:-localhost,127.0.0.1,::1}"
        export no_proxy="${no_proxy:-localhost,127.0.0.1,::1}"
    fi
}

validate_port() {
    local name="$1" value="$2"
    [[ "$value" =~ ^[0-9]+$ ]] || die "$name must be a number: $value"
    (( value >= 1 && value <= 65535 )) || die "$name must be between 1 and 65535: $value"
}

validate_runtime_config() {
    validate_port "backend port" "$BACKEND_PORT"
    validate_port "frontend port" "$FRONTEND_PORT"
    [[ "$BACKEND_PORT" != "$FRONTEND_PORT" ]] || die "backend and frontend ports must be different"
}

detect_runtime_os() {
    local os arch
    os="$(uname -s)"
    arch="$(uname -m)"
    case "$os" in
        Darwin) echo "macOS / $arch" ;;
        Linux)
            if [[ -f /etc/os-release ]]; then
                # shellcheck disable=SC1091
                . /etc/os-release
                echo "${PRETTY_NAME:-Linux} / $arch"
            else
                echo "Linux / $arch"
            fi
            ;;
        MINGW*|MSYS*|CYGWIN*) echo "Windows shell / $arch (Docker Desktop or WSL2 recommended)" ;;
        *) echo "$os / $arch" ;;
    esac
}

# ─── Process Management ───────────────────────────────────────
save_pid() {
    local name="$1" pid="$2"
    mkdir -p "$PID_DIR"
    echo "$pid" > "$PID_DIR/$name.pid"
}

read_pid() {
    local name="$1"
    local pidfile="$PID_DIR/$name.pid"
    if [[ -f "$pidfile" ]]; then
        cat "$pidfile"
    fi
}

is_running() {
    local name="$1"
    local pid
    pid=$(read_pid "$name")
    if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
        return 0
    fi
    return 1
}

port_listener() {
    local port="$1"
    if has lsof; then
        lsof -nP -iTCP:"$port" -sTCP:LISTEN 2>/dev/null | awk 'NR==2 {print $1 " PID " $2}'
    elif has netstat; then
        netstat -ano 2>/dev/null | grep -E ":${port} .*LISTEN" | head -1
    fi
}

stop_process() {
    local name="$1"
    local pid
    pid=$(read_pid "$name")
    if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
        info "Stopping $name (PID $pid)..."
        kill "$pid" 2>/dev/null || true
        # Wait briefly, then force kill
        sleep 1
        kill -0 "$pid" 2>/dev/null && kill -9 "$pid" 2>/dev/null || true
        rm -f "$PID_DIR/$name.pid"
        ok "$name stopped"
    else
        rm -f "$PID_DIR/$name.pid"
    fi
}

cleanup() {
    echo ""
    info "Shutting down..."
    stop_process "frontend"
    stop_process "square-monitor"
    stop_process "backend"
    ok "All services stopped"
    exit 0
}

# ─── Health Check ──────────────────────────────────────────────
wait_for_backend() {
    local max_attempts="${AIT_BACKEND_HEALTH_WAIT_SECONDS:-120}"
    local attempt=1

    info "Waiting for backend on :$BACKEND_PORT..."
    while [[ $attempt -le $max_attempts ]]; do
        if curl -s "http://localhost:$BACKEND_PORT/api/health" &>/dev/null; then
            ok "Backend is ready"
            return 0
        fi
        sleep 1
        ((attempt++))
    done

    warn "Backend did not respond after ${max_attempts}s — check logs"
    return 1
}

# ═══════════════════════════════════════════════════════════════
#  DEV MODE
# ═══════════════════════════════════════════════════════════════
dev_start() {
    step "Starting AiT in development mode"

    mkdir -p "$PID_DIR" "$LOG_DIR"
    load_env
    validate_runtime_config
    export_runtime_env

    # ── Backend ──
    info "Starting Go backend on :$BACKEND_PORT..."
    cd "$PROJECT_DIR"
    go run main.go \
        > "$LOG_DIR/backend.log" 2>&1 &
    save_pid "backend" $!
    ok "Backend started (PID $!, log: .logs/backend.log)"

    # Wait for backend to be healthy before starting frontend
    wait_for_backend

    # ── Frontend ──
    info "Starting frontend dev server on :$FRONTEND_PORT..."
    cd "$PROJECT_DIR/web"
    VITE_PORT=$FRONTEND_PORT npm run dev \
        > "$LOG_DIR/frontend.log" 2>&1 &
    save_pid "frontend" $!
    cd "$PROJECT_DIR"
    ok "Frontend started (PID $!, log: .logs/frontend.log)"

    # ── Square Monitor (optional) ──
    local square_dir="$PROJECT_DIR/scripts/square-monitor"
    if [[ "${AIT_SKIP_SQUARE:-0}" != "1" ]] && [[ -d "$square_dir" ]]; then
        if [[ -f "$square_dir/.venv/bin/python" ]]; then
            info "Starting Square Monitor on :8000..."
            cd "$square_dir"
            .venv/bin/python web.py \
                > "$LOG_DIR/square-monitor.log" 2>&1 &
            save_pid "square-monitor" $!
            cd "$PROJECT_DIR"
            ok "Square Monitor started (PID $!, log: .logs/square-monitor.log)"
        else
            info "Square Monitor skipped (run: scripts/install.sh to set up)"
        fi
    fi

    divider
    echo ""
    echo -e "${GREEN}${BOLD}  AiT is running!${NC}"
    echo ""
    echo -e "  ${BOLD}Web Dashboard:${NC}     http://localhost:$FRONTEND_PORT"
    echo -e "  ${BOLD}API Endpoint:${NC}      http://localhost:$BACKEND_PORT"
    echo -e "  ${BOLD}Square Monitor:${NC}    http://localhost:8000"
    echo ""
    echo -e "  ${DIM}Logs:${NC}   tail -f .logs/backend.log"
    echo -e "  ${DIM}Stop:${NC}   Ctrl+C or ./scripts/start.sh stop"
    echo ""

    # Trap Ctrl+C
    trap cleanup INT TERM

    # Keep script alive
    info "Press Ctrl+C to stop all services"
    wait
}

# ═══════════════════════════════════════════════════════════════
#  DOCKER MODE
# ═══════════════════════════════════════════════════════════════
detect_compose() {
    if docker compose version &>/dev/null; then
        COMPOSE_CMD="docker compose"
    elif has docker-compose; then
        COMPOSE_CMD="docker-compose"
    else
        die "Docker Compose not found"
    fi

    COMPOSE_FILE="${AIT_COMPOSE_FILE:-}"
    if [[ -z "$COMPOSE_FILE" ]]; then
        if [[ -f "$PROJECT_DIR/docker-compose.prod.yml" ]]; then
            COMPOSE_FILE="docker-compose.prod.yml"
        else
            COMPOSE_FILE="docker-compose.yml"
        fi
    fi
    COMPOSE_ARGS=(-f "$COMPOSE_FILE")
    info "Docker Compose file: $COMPOSE_FILE"
}

docker_start() {
    step "Starting Docker services"
    detect_compose
    load_env
    validate_runtime_config
    export_runtime_env
    mkdir -p "$PROJECT_DIR/data"

    cd "$PROJECT_DIR"

    if [[ "${1:-}" == "--build" ]]; then
        $COMPOSE_CMD "${COMPOSE_ARGS[@]}" up -d --build
    else
        $COMPOSE_CMD "${COMPOSE_ARGS[@]}" up -d
    fi

    divider
    echo ""
    echo -e "${GREEN}${BOLD}  AiT Docker services started!${NC}"
    echo ""
    echo -e "  ${BOLD}Web Dashboard:${NC}  http://localhost:$FRONTEND_PORT"
    echo -e "  ${BOLD}API Endpoint:${NC}   http://localhost:$BACKEND_PORT"
    echo ""
    echo -e "  ${DIM}Logs:${NC}   ./scripts/start.sh docker logs"
    echo -e "  ${DIM}Stop:${NC}   ./scripts/start.sh docker stop"
    echo ""
}

docker_stop() {
    detect_compose
    load_env
    export_runtime_env
    cd "$PROJECT_DIR"
    info "Stopping Docker services..."
    $COMPOSE_CMD "${COMPOSE_ARGS[@]}" stop
    ok "Services stopped"
}

docker_restart() {
    detect_compose
    load_env
    export_runtime_env
    cd "$PROJECT_DIR"
    info "Restarting Docker services..."
    $COMPOSE_CMD "${COMPOSE_ARGS[@]}" restart
    ok "Services restarted"
}

docker_logs() {
    detect_compose
    load_env
    cd "$PROJECT_DIR"
    if [[ -n "${1:-}" ]]; then
        $COMPOSE_CMD "${COMPOSE_ARGS[@]}" logs -f "$1"
    else
        $COMPOSE_CMD "${COMPOSE_ARGS[@]}" logs -f
    fi
}

docker_status() {
    detect_compose
    load_env
    validate_runtime_config
    cd "$PROJECT_DIR"
    info "Docker service status:"
    $COMPOSE_CMD "${COMPOSE_ARGS[@]}" ps
    echo ""
    info "Health check:"
    curl -s "http://localhost:$BACKEND_PORT/api/health" 2>/dev/null | head -c 200 || echo "Backend not responding"
    echo ""
}

docker_clean() {
    detect_compose
    cd "$PROJECT_DIR"
    warn "This will stop and remove all containers and volumes!"
    read -p "Confirm? (yes/no): " confirm
    if [[ "$confirm" == "yes" ]]; then
        $COMPOSE_CMD "${COMPOSE_ARGS[@]}" down -v
        ok "Cleanup complete"
    else
        info "Cancelled"
    fi
}

docker_update() {
    detect_compose
    cd "$PROJECT_DIR"
    info "Pulling latest code..."
    git pull --ff-only
    info "Rebuilding and restarting..."
    $COMPOSE_CMD "${COMPOSE_ARGS[@]}" up -d --build
    ok "Updated and running"
}

docker_regenerate_keys() {
    cd "$PROJECT_DIR"
    warn "This will regenerate ALL encryption keys!"
    warn "Existing encrypted data will become unreadable!"
    read -p "Confirm? (yes/no): " confirm
    if [[ "$confirm" != "yes" ]]; then
        info "Cancelled"
        return
    fi

    local env_file=".env"
    [[ -f "$env_file" ]] || die ".env not found"

    local jwt_secret data_key rsa_key
    jwt_secret=$(openssl rand -base64 32)
    data_key=$(openssl rand -base64 32)
    rsa_key=$(openssl genrsa 2048 2>/dev/null | awk '{printf "%s\\n", $0}')

    _set_env "JWT_SECRET" "$jwt_secret"
    _set_env "DATA_ENCRYPTION_KEY" "$data_key"
    _set_env "RSA_PRIVATE_KEY" "\"$rsa_key\""
    chmod 600 "$env_file"

    ok "All keys regenerated"
    warn "Restart services: ./scripts/start.sh docker restart"
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

# ═══════════════════════════════════════════════════════════════
#  PROD MODE
# ═══════════════════════════════════════════════════════════════
prod_start() {
    step "Starting AiT in production mode"

    mkdir -p "$PID_DIR" "$LOG_DIR"
    load_env
    validate_runtime_config
    export_runtime_env

    cd "$PROJECT_DIR"

    # Build if binary doesn't exist or source is newer
    if [[ ! -f "./ait" ]] || [[ "main.go" -nt "./ait" ]]; then
        info "Building backend..."
        CGO_ENABLED=1 go build -o ait .
        ok "Backend built"
    fi

    # Build frontend if dist doesn't exist
    if [[ ! -d "web/dist" ]]; then
        info "Building frontend..."
        cd web && npm run build 2>&1 | tail -1 && cd ..
        ok "Frontend built"
    fi

    # Start backend
    info "Starting backend on :$BACKEND_PORT..."
    nohup ./ait > "$LOG_DIR/backend.log" 2>&1 &
    save_pid "backend" $!
    ok "Backend started (PID $!)"

    # Start a simple static server for frontend (or use nginx if available)
    if has nginx; then
        info "Frontend served via nginx (configure separately)"
    else
        info "Starting frontend static server on :$FRONTEND_PORT..."
        cd "$PROJECT_DIR/web"
        npx serve dist -l "$FRONTEND_PORT" -s \
            > "$LOG_DIR/frontend.log" 2>&1 &
        save_pid "frontend" $!
        cd "$PROJECT_DIR"
        ok "Frontend started (PID $!)"
    fi

    divider
    echo ""
    echo -e "${GREEN}${BOLD}  AiT production services started!${NC}"
    echo ""
    echo -e "  ${BOLD}Web Dashboard:${NC}  http://localhost:$FRONTEND_PORT"
    echo -e "  ${BOLD}API Endpoint:${NC}   http://localhost:$BACKEND_PORT"
    echo ""
    echo -e "  ${DIM}Stop:${NC}   ./scripts/start.sh prod stop"
    echo -e "  ${DIM}Logs:${NC}   tail -f .logs/backend.log"
    echo ""
}

# ═══════════════════════════════════════════════════════════════
#  STOP (dev/prod)
# ═══════════════════════════════════════════════════════════════
dev_stop() {
    step "Stopping AiT services"
    stop_process "frontend"
    stop_process "square-monitor"
    stop_process "backend"
    ok "All services stopped"
}

# ═══════════════════════════════════════════════════════════════
#  STATUS (dev/prod)
# ═══════════════════════════════════════════════════════════════
dev_status() {
    load_env
    step "AiT Service Status"

    echo -e "  ${BOLD}System:${NC}   $(detect_runtime_os)"
    echo -e "  ${BOLD}Project:${NC}  $PROJECT_DIR"
    echo -e "  ${BOLD}Ports:${NC}    backend=$BACKEND_PORT frontend=$FRONTEND_PORT"
    if [[ -n "$PROXY_URL" ]]; then
        echo -e "  ${BOLD}Proxy:${NC}    $PROXY_URL"
    else
        echo -e "  ${BOLD}Proxy:${NC}    not configured"
    fi
    echo ""

    for svc in backend frontend square-monitor; do
        local pid
        pid=$(read_pid "$svc")
        if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
            echo -e "  ${GREEN}●${NC} $svc  (PID $pid)"
        else
            echo -e "  ${DIM}○${NC} $svc  (not running)"
        fi
    done

    echo ""
    info "Port listeners:"
    local backend_listener frontend_listener
    backend_listener="$(port_listener "$BACKEND_PORT" || true)"
    frontend_listener="$(port_listener "$FRONTEND_PORT" || true)"
    if [[ -n "$backend_listener" ]]; then
        echo -e "  ${GREEN}●${NC} backend port :$BACKEND_PORT  ($backend_listener)"
    else
        echo -e "  ${DIM}○${NC} backend port :$BACKEND_PORT  (not listening)"
    fi
    if [[ -n "$frontend_listener" ]]; then
        echo -e "  ${GREEN}●${NC} frontend port :$FRONTEND_PORT ($frontend_listener)"
    else
        echo -e "  ${DIM}○${NC} frontend port :$FRONTEND_PORT (not listening)"
    fi

    echo ""
    info "HTTP health:"
    curl -s "http://localhost:$BACKEND_PORT/api/health" 2>/dev/null | head -c 200 || echo "  Backend not responding"
    echo ""
    curl -sI "http://localhost:$FRONTEND_PORT" 2>/dev/null | head -1 || echo "  Frontend not responding"
    echo ""
}

# ═══════════════════════════════════════════════════════════════
#  LOGS / UPDATE / CONFIG / UNINSTALL / DIAGNOSTICS
# ═══════════════════════════════════════════════════════════════
dev_logs() {
    local svc="${1:-backend}"
    local file="$LOG_DIR/$svc.log"
    [[ -f "$file" ]] || die "Log file not found: $file"
    info "Tailing $file (Ctrl+C to exit)"
    tail -f "$file"
}

restart_mode() {
    local mode="${1:-dev}"
    case "$mode" in
        docker|compose) docker_restart ;;
        prod|production)
            dev_stop
            prod_start
            ;;
        dev|development)
            dev_stop
            dev_start
            ;;
        *) die "Unknown restart mode: $mode" ;;
    esac
}

update_project() {
    local mode="${1:-dev}"
    step "Updating AiT"
    load_env
    export_runtime_env
    cd "$PROJECT_DIR"

    info "1/5 Checking git worktree"
    if [[ -d ".git" ]]; then
        git status --short
        info "Pulling latest code with fast-forward only"
        git pull --ff-only || warn "git pull failed; local changes may need manual review"
    else
        warn "Not a git checkout; skipping git pull"
    fi

    if [[ "$mode" == "docker" || "$mode" == "compose" ]]; then
        detect_compose
        info "2/5 Pulling/rebuilding Docker services"
        $COMPOSE_CMD pull || true
        $COMPOSE_CMD up -d --build
        ok "Docker update complete"
        return
    fi

    info "2/5 Downloading Go modules"
    go mod download
    info "3/5 Installing frontend packages"
    (cd web && npm install --no-fund --no-audit)
    info "4/5 Building backend"
    CGO_ENABLED=1 go build -o ait .
    info "5/5 Building frontend"
    (cd web && npm run build)
    ok "Update/build complete"
    warn "Restart services to use the new build: ./scripts/start.sh prod restart"
}

configure_runtime() {
    step "Configure AiT ports/proxy"
    load_env
    mkdir -p "$PROJECT_DIR"
    cd "$PROJECT_DIR"

    echo "Current backend port: $BACKEND_PORT"
    read -r -p "Backend API port [${BACKEND_PORT}]: " new_backend
    new_backend="${new_backend:-$BACKEND_PORT}"

    echo "Current frontend port: $FRONTEND_PORT"
    read -r -p "Frontend dashboard port [${FRONTEND_PORT}]: " new_frontend
    new_frontend="${new_frontend:-$FRONTEND_PORT}"

    echo "Current proxy: ${PROXY_URL:-not configured}"
    read -r -p "Proxy URL, blank to clear [${PROXY_URL}]: " new_proxy
    if [[ -z "$new_proxy" && -n "$PROXY_URL" ]]; then
        read -r -p "Clear existing proxy? (yes/no) [no]: " clear_proxy
        [[ "$clear_proxy" == "yes" ]] && new_proxy=""
        [[ "$clear_proxy" != "yes" ]] && new_proxy="$PROXY_URL"
    fi

    validate_port "backend port" "$new_backend"
    validate_port "frontend port" "$new_frontend"
    [[ "$new_backend" != "$new_frontend" ]] || die "backend and frontend ports must be different"

    [[ -f ".env" ]] || touch .env
    _set_env "AIT_BACKEND_PORT" "$new_backend"
    _set_env "AIT_FRONTEND_PORT" "$new_frontend"
    _set_env "AIT_TIMEZONE" "$TIMEZONE"
    _set_env "AIT_PROXY_URL" "$new_proxy"
    _set_env "AIT_BINANCE_PROXY_URL" "$new_proxy"
    _set_env "HTTP_PROXY" "$new_proxy"
    _set_env "HTTPS_PROXY" "$new_proxy"
    _set_env "ALL_PROXY" "$new_proxy"
    _set_env "NO_PROXY" "localhost,127.0.0.1,::1"
    chmod 600 .env 2>/dev/null || true

    ok "Configuration saved to .env"
    warn "Restart AiT for port/proxy changes to take effect"
}

backup_runtime() {
    step "Backing up AiT runtime data"
    cd "$PROJECT_DIR"
    mkdir -p backups
    local stamp out
    stamp="$(date +%Y%m%d-%H%M%S)"
    out="backups/ait-backup-${stamp}.tar.gz"
    tar -czf "$out" .env data 2>/dev/null || tar -czf "$out" data
    ok "Backup created: $out"
}

uninstall_ait() {
    step "Uninstall AiT runtime services"
    warn "This stops services and removes generated runtime/build artifacts."
    warn "By default it preserves .env and data/ so accounts, keys and history are not lost."
    read -r -p "Continue uninstall? (yes/no): " confirm
    [[ "$confirm" == "yes" ]] || { info "Cancelled"; return; }

    dev_stop || true
    if has docker; then
        if docker compose version &>/dev/null || has docker-compose; then
            detect_compose
            cd "$PROJECT_DIR"
            $COMPOSE_CMD "${COMPOSE_ARGS[@]}" down 2>/dev/null || true
        fi
    fi

    cd "$PROJECT_DIR"
    rm -rf .pids .logs ait web/dist 2>/dev/null || true
    ok "Runtime/build artifacts removed"

    read -r -p "Also remove node_modules? (yes/no) [no]: " remove_modules
    if [[ "$remove_modules" == "yes" ]]; then
        rm -rf web/node_modules node_modules 2>/dev/null || true
        ok "node_modules removed"
    fi

    read -r -p "DANGER: remove .env and data/? Type DELETE to confirm: " remove_data
    if [[ "$remove_data" == "DELETE" ]]; then
        rm -rf .env data 2>/dev/null || true
        ok ".env and data/ removed"
    else
        info "Preserved .env and data/"
    fi
}

diagnose_runtime() {
    step "AiT diagnostics"
    load_env
    echo -e "${BOLD}System:${NC} $(detect_runtime_os)"
    echo -e "${BOLD}Project:${NC} $PROJECT_DIR"
    echo -e "${BOLD}Backend:${NC} http://localhost:$BACKEND_PORT/api/health"
    echo -e "${BOLD}Frontend:${NC} http://localhost:$FRONTEND_PORT"
    echo -e "${BOLD}Proxy:${NC} ${PROXY_URL:-not configured}"
    echo ""

    info "Toolchain"
    for cmd in git go node npm docker curl; do
        if has "$cmd"; then
            echo "  $cmd: $($cmd --version 2>/dev/null | head -1)"
        else
            echo "  $cmd: not found"
        fi
    done

    echo ""
    info "Ports"
    if has lsof; then
        lsof -nP -iTCP:"$BACKEND_PORT" -sTCP:LISTEN 2>/dev/null || echo "  backend port not listening"
        lsof -nP -iTCP:"$FRONTEND_PORT" -sTCP:LISTEN 2>/dev/null || echo "  frontend port not listening"
    elif has netstat; then
        netstat -ano 2>/dev/null | grep -E ":(${BACKEND_PORT}|${FRONTEND_PORT})" || true
    else
        warn "Neither lsof nor netstat found; skipping port process lookup"
    fi

    echo ""
    info "Backend health"
    curl -s "http://localhost:$BACKEND_PORT/api/health" 2>/dev/null || echo "  Backend not responding"
    echo ""
}

# ═══════════════════════════════════════════════════════════════
#  INTERACTIVE MENU
# ═══════════════════════════════════════════════════════════════
show_menu() {
    while true; do
        load_env
        clear 2>/dev/null || true
        echo -e "${CYAN}${BOLD}"
        echo "  ╔═══════════════════════════════════════════════════╗"
        echo "  ║              AiT 管理菜单 / Menu                  ║"
        echo "  ╚═══════════════════════════════════════════════════╝"
        echo -e "${NC}"
        echo -e "  系统: $(detect_runtime_os)"
        echo -e "  项目: $PROJECT_DIR"
        echo -e "  前端: http://localhost:$FRONTEND_PORT"
        echo -e "  后端: http://localhost:$BACKEND_PORT"
        echo -e "  代理: ${PROXY_URL:-未配置}"
        echo ""
        echo "  1) 一键部署/修复依赖（dev 源码模式）"
        echo "  2) 一键部署/启动（Docker 模式）"
        echo "  3) 启动 AIT 服务（dev）"
        echo "  4) 启动 AIT 服务（prod）"
        echo "  5) 查询前后端运行状态"
        echo "  6) 查看日志"
        echo "  7) 重启 AIT 服务"
        echo "  8) 停止 AIT 服务"
        echo "  9) 更新 AIT 并重新构建"
        echo " 10) 设置前后端端口 / 代理 IP"
        echo " 11) 备份 .env 与 data/"
        echo " 12) 环境诊断 / 端口占用 / 健康检查"
        echo " 13) 卸载 AIT 运行产物"
        echo "  0) 退出"
        echo ""
        read -r -p "请选择操作: " choice
        echo ""
        case "$choice" in
            1)
                "$PROJECT_DIR/scripts/install.sh" --dev --dir "$PROJECT_DIR"
                ;;
            2)
                "$PROJECT_DIR/scripts/install.sh" --docker --dir "$PROJECT_DIR"
                ;;
            3) dev_start ;;
            4) prod_start ;;
            5) dev_status ;;
            6)
                read -r -p "日志名称 backend/frontend/square-monitor [backend]: " svc
                dev_logs "${svc:-backend}"
                ;;
            7)
                read -r -p "重启模式 dev/prod/docker [prod]: " mode
                restart_mode "${mode:-prod}"
                ;;
            8)
                read -r -p "停止模式 dev/prod/docker [dev]: " mode
                if [[ "${mode:-dev}" == "docker" ]]; then docker_stop; else dev_stop; fi
                ;;
            9)
                read -r -p "更新模式 dev/docker [dev]: " mode
                update_project "${mode:-dev}"
                ;;
            10) configure_runtime ;;
            11) backup_runtime ;;
            12) diagnose_runtime ;;
            13) uninstall_ait ;;
            0) exit 0 ;;
            *) warn "Unknown choice: $choice" ;;
        esac
        echo ""
        read -r -p "按 Enter 返回菜单..." _
    done
}

# ═══════════════════════════════════════════════════════════════
#  HELP
# ═══════════════════════════════════════════════════════════════
show_help() {
    echo -e "${BOLD}AiT Startup Script${NC}"
    echo ""
    echo "Usage: ./scripts/start.sh [mode] [command]"
    echo ""
    echo -e "${BOLD}Modes:${NC}"
    echo "  dev        Development mode (default)"
    echo "  docker     Docker Compose management"
    echo "  prod       Production mode (build + run)"
    echo "  menu       Interactive management menu"
    echo ""
    echo -e "${BOLD}Commands:${NC}"
    echo "  start      Start services (default)"
    echo "  stop       Stop services"
    echo "  restart    Restart services"
    echo "  status     Show service status"
    echo "  logs       View logs (docker mode)"
    echo "  update     Pull latest code and rebuild"
    echo "  config     Configure ports/proxy in .env"
    echo "  backup     Backup .env and data/"
    echo "  diagnose   Environment, port and health checks"
    echo "  uninstall  Stop services and remove runtime/build artifacts"
    echo ""
    echo -e "${BOLD}Docker-only commands:${NC}"
    echo "  clean      Remove containers and volumes"
    echo "  update     Pull latest and rebuild"
    echo "  regenerate-keys  Regenerate encryption keys"
    echo ""
    echo -e "${BOLD}Examples:${NC}"
    echo "  ./scripts/start.sh              # Dev mode"
    echo "  ./scripts/start.sh dev stop     # Stop dev services"
    echo "  ./scripts/start.sh docker       # Docker mode"
    echo "  ./scripts/start.sh docker logs  # Docker logs"
    echo "  ./scripts/start.sh prod         # Production mode"
    echo "  ./scripts/start.sh menu         # Interactive menu"
    echo ""
    echo -e "${BOLD}Environment:${NC}"
    echo "  AIT_BACKEND_PORT=8080   Backend API port"
    echo "  AIT_FRONTEND_PORT=3000  Frontend port"
    echo "  AIT_PROXY_URL=http://127.0.0.1:7897  Proxy for dependencies/exchange HTTP"
    echo "  AIT_BINANCE_PROXY_URL=...             Binance market-data proxy"
    echo "  AIT_SKIP_SQUARE=1       Skip Square Monitor"
}

# ═══════════════════════════════════════════════════════════════
#  MAIN
# ═══════════════════════════════════════════════════════════════
main() {
    local mode="${1:-dev}"
    local cmd="${2:-start}"

    case "$mode" in
        dev|development)
            case "$cmd" in
                start)      dev_start ;;
                stop)       dev_stop ;;
                restart)    restart_mode dev ;;
                status)     dev_status ;;
                logs|log)   dev_logs "${3:-backend}" ;;
                update)     update_project dev ;;
                config)     configure_runtime ;;
                backup)     backup_runtime ;;
                diagnose)   diagnose_runtime ;;
                uninstall)  uninstall_ait ;;
                *)          die "Unknown command: $cmd" ;;
            esac
            ;;
        docker|compose)
            case "$cmd" in
                start)            docker_start "${3:-}" ;;
                stop)             docker_stop ;;
                restart)          docker_restart ;;
                logs|log)         docker_logs "${3:-}" ;;
                status)           docker_status ;;
                clean)            docker_clean ;;
                update)           docker_update ;;
                config)           configure_runtime ;;
                backup)           backup_runtime ;;
                diagnose)         diagnose_runtime ;;
                uninstall)        uninstall_ait ;;
                regenerate-keys)  docker_regenerate_keys ;;
                *)                die "Unknown command: $cmd" ;;
            esac
            ;;
        prod|production)
            case "$cmd" in
                start)      prod_start ;;
                stop)       dev_stop ;;
                restart)    restart_mode prod ;;
                status)     dev_status ;;
                logs|log)   dev_logs "${3:-backend}" ;;
                update)     update_project prod ;;
                config)     configure_runtime ;;
                backup)     backup_runtime ;;
                diagnose)   diagnose_runtime ;;
                uninstall)  uninstall_ait ;;
                *)          die "Unknown command: $cmd" ;;
            esac
            ;;
        menu)
            show_menu
            ;;
        stop)
            dev_stop
            ;;
        status)
            dev_status
            ;;
        restart)
            restart_mode prod
            ;;
        logs|log)
            dev_logs "${2:-backend}"
            ;;
        update)
            update_project dev
            ;;
        config)
            configure_runtime
            ;;
        backup)
            backup_runtime
            ;;
        diagnose)
            diagnose_runtime
            ;;
        uninstall)
            uninstall_ait
            ;;
        help|--help|-h)
            show_help
            ;;
        *)
            die "Unknown mode: $mode (try: dev, docker, prod)"
            ;;
    esac
}

main "$@"
