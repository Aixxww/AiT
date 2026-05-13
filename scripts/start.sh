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
}

# ─── Env var migration (NOFX_ → AIT_) ─────────────────────────
migrate_env_prefix() {
    local env_file="$PROJECT_DIR/.env"
    [[ -f "$env_file" ]] || return 0

    if grep -q "^NOFX_" "$env_file" 2>/dev/null; then
        info "Migrating NOFX_ → AIT_ prefix in .env..."
        local tmp
        tmp=$(sed 's/^NOFX_/AIT_/g' "$env_file")
        echo "$tmp" > "$env_file"
        ok "Environment variables migrated"
    fi
}

# ─── Port config ───────────────────────────────────────────────
BACKEND_PORT="${AIT_BACKEND_PORT:-${NOFX_BACKEND_PORT:-8080}}"
FRONTEND_PORT="${AIT_FRONTEND_PORT:-${NOFX_FRONTEND_PORT:-3000}}"
TIMEZONE="${AIT_TIMEZONE:-${NOFX_TIMEZONE:-Asia/Shanghai}}"

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
    local max_attempts=30
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

    # ── Backend ──
    info "Starting Go backend on :$BACKEND_PORT..."
    cd "$PROJECT_DIR"
    API_SERVER_PORT=$BACKEND_PORT TZ="$TIMEZONE" \
        go run main.go \
        > "$LOG_DIR/backend.log" 2>&1 &
    save_pid "backend" $!
    ok "Backend started (PID $!, log: .logs/backend.log)"

    # Wait for backend to be healthy before starting frontend
    wait_for_backend

    # ── Frontend ──
    info "Starting frontend dev server on :$FRONTEND_PORT..."
    cd "$PROJECT_DIR/web"
    VITE_PORT=$FRONTEND_PORT \
        npm run dev \
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
}

docker_start() {
    step "Starting Docker services"
    detect_compose
    load_env
    mkdir -p "$PROJECT_DIR/data"

    cd "$PROJECT_DIR"

    if [[ "${1:-}" == "--build" ]]; then
        $COMPOSE_CMD up -d --build
    else
        $COMPOSE_CMD up -d
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
    cd "$PROJECT_DIR"
    info "Stopping Docker services..."
    $COMPOSE_CMD stop
    ok "Services stopped"
}

docker_restart() {
    detect_compose
    cd "$PROJECT_DIR"
    info "Restarting Docker services..."
    $COMPOSE_CMD restart
    ok "Services restarted"
}

docker_logs() {
    detect_compose
    cd "$PROJECT_DIR"
    if [[ -n "${2:-}" ]]; then
        $COMPOSE_CMD logs -f "$2"
    else
        $COMPOSE_CMD logs -f
    fi
}

docker_status() {
    detect_compose
    load_env
    cd "$PROJECT_DIR"
    info "Docker service status:"
    $COMPOSE_CMD ps
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
        $COMPOSE_CMD down -v
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
    $COMPOSE_CMD up -d --build
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
    API_SERVER_PORT=$BACKEND_PORT TZ="$TIMEZONE" \
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
    info "Health check:"
    curl -s "http://localhost:$BACKEND_PORT/api/health" 2>/dev/null | head -c 200 || echo "  Backend not responding"
    echo ""
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
    echo ""
    echo -e "${BOLD}Commands:${NC}"
    echo "  start      Start services (default)"
    echo "  stop       Stop services"
    echo "  restart    Restart services"
    echo "  status     Show service status"
    echo "  logs       View logs (docker mode)"
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
    echo ""
    echo -e "${BOLD}Environment:${NC}"
    echo "  AIT_BACKEND_PORT=8080   Backend API port"
    echo "  AIT_FRONTEND_PORT=3000  Frontend port"
    echo "  AIT_SKIP_SQUARE=1       Skip Square Monitor"
}

# ═══════════════════════════════════════════════════════════════
#  MAIN
# ═══════════════════════════════════════════════════════════════
main() {
    local mode="${1:-dev}"
    local cmd="${2:-start}"

    # Migrate old env prefix
    migrate_env_prefix

    case "$mode" in
        dev|development)
            case "$cmd" in
                start)  dev_start ;;
                stop)   dev_stop ;;
                status) dev_status ;;
                *)      die "Unknown command: $cmd" ;;
            esac
            ;;
        docker|compose)
            case "$cmd" in
                start)            docker_start "${3:-}" ;;
                stop)             docker_stop ;;
                restart)          docker_restart ;;
                logs|log)         docker_logs "$@" ;;
                status)           docker_status ;;
                clean)            docker_clean ;;
                update)           docker_update ;;
                regenerate-keys)  docker_regenerate_keys ;;
                *)                die "Unknown command: $cmd" ;;
            esac
            ;;
        prod|production)
            case "$cmd" in
                start)  prod_start ;;
                stop)   dev_stop ;;
                status) dev_status ;;
                *)      die "Unknown command: $cmd" ;;
            esac
            ;;
        stop)
            dev_stop
            ;;
        status)
            dev_status
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
