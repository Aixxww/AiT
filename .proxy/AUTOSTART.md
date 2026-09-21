# AiT stack boot autostart + sing-box watchdog

Installed on this Grok Bot box to keep the AiT stack alive across container
start and hibernate wake (same pattern as Tailscale).

## What was installed

| Piece | Path |
|-------|------|
| Wrapper / watchdog | `/usr/local/bin/ait-stack-autostart` |
| Boot hook | `/usr/local/bin/start-sand-box` (block `# >>> ait-stack-autostart`) |
| Backup of start-sand-box | `/usr/local/bin/start-sand-box.bak-before-ait` |
| Copy under workspace | `/workspace/AiT/.proxy/ait-stack-autostart` |
| Logs | `/workspace/AiT/.logs/ait-stack-autostart.log` (+ per-component logs) |

Secrets stay in existing files (`sing-box-trojan.json`, `/workspace/AiT/.env`); the
wrapper never prints them.

## Usage

```bash
# One-shot: start anything unhealthy; leave healthy processes alone
/usr/local/bin/ait-stack-autostart

# Watchdog: same, then loop every ~20s; flock so only one runs
/usr/local/bin/ait-stack-autostart --watch
```

On boot / hibernate wake, `start-sand-box` launches:

`setsid nohup /usr/local/bin/ait-stack-autostart --watch`

(as `box` when `SAND_USER_NON_ROOT` is set).

## Start order (health-checked)

1. **sing-box Trojan client** — `/workspace/bin/sing-box run -c /workspace/AiT/.proxy/sing-box-trojan.json` (10808 SOCKS / 10809 HTTP)
2. **AiT backend** — `/workspace/AiT/.proxy/start_backend.sh` → `:8080`
3. **Vite frontend** — `/workspace/AiT/web` vite `--host 0.0.0.0 --port 3000`
4. **Square Monitor** — `web.py` `:8000`, `worker.py`, `socat [::1]:8000` → `127.0.0.1:8000`  
   (`SQUARE_PROXY=http://127.0.0.1:10809`, venv under `scripts/square-monitor`)
5. **Tailscale Funnel** — re-assert `sudo tailscale funnel --bg --yes http://127.0.0.1:3000` if needed  

`tailscaled` itself keeps its own `/usr/local/bin/tailscaled-autostart` hook — do not remove it.

## Persistence warning

**Update Grok Bot's Computer** (box image / rootfs refresh) **still wipes**
`/usr/local/bin/ait-stack-autostart` and the `start-sand-box` patch. Workspace
files under `/workspace/AiT/.proxy/` may survive depending on volume layout;
re-install the wrapper into `/usr/local/bin` and re-apply the boot hook after
an update if the stack stops auto-starting.

## Env knobs

- `AIT_V7_PERSIST_TRACK` (default `1`; set `0` in `/workspace/AiT/.proxy/ait-stack.env` to disable hunter v7 persist/track autostart)
- `AIT_STACK_ENV` (default `/workspace/AiT/.proxy/ait-stack.env`)
- `AIT_STACK_WATCH_INTERVAL_S` (default `20`)
- `WATCH_LOCK` (default `/tmp/ait-stack-autostart.watch.lock`)
