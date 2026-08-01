# Speedtest

Eigenständige Netzwerk-Mess-Plattform: Go Backend + embedded Web-Frontend.

Internet-Speedtests via Ookla-Protokoll (`speedtest-go`), erweiterte Diagnostik
(Bufferbloat, Traceroute), SQLite-Historie, zeitgesteuerte Messungen per Cron-Expression.

## Quick Start

```bash
# Build
go build -o speedtest.exe ./cmd/speedtest/

# Run (default port 8080)
./speedtest.exe

# Custom port
SPEEDTEST_PORT=8090 ./speedtest.exe
```

Dann im Browser öffnen: `http://localhost:8080`

## Konfiguration

| Env-Variable | Default | Beschreibung |
|---|---|---|
| `SPEEDTEST_PORT` | `8080` | HTTP-Port |
| `SPEEDTEST_DB` | `speedtest.db` | Pfad zur SQLite-Datenbank |
| `TZ` | Systemzeitzone | Zeitzone, im Docker-Setup `Europe/Berlin` |

## Docker

Das Compose-Setup veröffentlicht das Dashboard auf `http://localhost:8088` und speichert SQLite dauerhaft im Volume `speedtest-data`.

```bash
# Direkt unter Linux/WSL
docker compose up -d --build

# Von Windows über die vorhandene WSL2-Docker-Engine
wsl.exe -d Ubuntu-24.04 -- sh -lc "cd /mnt/d/Repos/speedtest && docker compose up -d --build"

# Status und Logs
docker compose ps
docker compose logs --tail=100 speedtest

# Beenden; das Datenvolume bleibt erhalten
docker compose down
```

Der Container läuft als nicht privilegierter Benutzer, besitzt keine zusätzlichen Linux-Capabilities und verwendet `/data/speedtest.db`.

## Tech Stack

- **Go** 1.22+ (Backend, Single Binary)
- **speedtest-go** — Ookla Speedtest Protokoll
- **robfig/cron/v3** — Scheduler
- **modernc.org/sqlite** — Pure-Go SQLite (kein CGO)
- **Vanilla JS + Chart.js** — Frontend (embedded)
