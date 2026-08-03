# Speedtest

Selbsthostbare Netzwerk-Messplattform in Go — misst vollautomatisch deine Internetleitung und vergleicht die Ergebnisse mit deinem Vertrag.

Single Binary, keine externen Abhängigkeiten, eingebautes Web-Dashboard.

---

## Features

- **Internet-Speedtest** via Ookla-Protokoll (`speedtest-go`)
- **Bufferbloat-Analyse** (ICMP + HTTP-Fallback)
- **Traceroute** mit Hop-Darstellung
- **SQLite-Historie** mit trend-Analyse über konfigurierbare Zeiträume (3/5/7/14/30 Tage)
- **Cron-Scheduler** für vollautomatische Hintergrundmessungen — läuft auch ohne offenes Browser-Fenster
- **Tarif-Soll/Ist-Vergleich** mit vorausgefülltem Anbieter-/Tarifkatalog (Telekom, Vodafone, O2, 1&1, Deutsche Glasfaser)
  - Persistierte Abweichungssnapshots pro Messung
  - Automatische Backfill-Migration für bestehende Daten
  - Tarifversionierung — alte Messungen bleiben korrekt zugeordnet
  - Live-Warnung bei Unterschreitung mit konsekutiver Werktag-Strähne
  - **Mängelmeldung-Generator (§41 TKG)** — erstellt ein formales Beschwerdeschreiben,
    wenn die gesetzliche Schwelle erfüllt ist (mindestens 2 aufeinanderfolgende Werktage).
    Nutzung auf eigene Verantwortung — keine Rechtsberatung.
- **Live-Dashboard** mit SSE-Push (Server-Sent Events) für Echtzeit-Updates während des Tests
- **Dark-Mode** UI mit Ring-Charts, Zeitreihen und Today-Verlauf
- **Docker-Support** inkl. Dockerfile und Compose-Setup
- **Graceful Shutdown**, HTTP-Server-Timeouts, Request-Logging
- **Toast-Notifications** statt blockierender `alert()`-Dialoge
- **Daten-Retention** option

## Quick Start

```bash
# Build
go build -o speedtest ./cmd/speedtest/

# Run (default port 8080)
./speedtest

# Custom port
SPEEDTEST_PORT=8090 ./speedtest
```

Dann im Browser öffnen: `http://localhost:8080`

## Konfiguration

| Env-Variable | Default | Beschreibung |
|---|---|---|
| `SPEEDTEST_PORT` | `8080` | HTTP-Port |
| `SPEEDTEST_DB` | `speedtest.db` | Pfad zur SQLite-Datenbank |
| `SPEEDTEST_RETENTION_DAYS` | _(deaktiviert)_ | Ergebnisse älter als N Tage automatisch löschen |
| `TZ` | Systemzeitzone | Zeitzone, im Docker-Setup `Europe/Berlin` |

## Docker

Das Compose-Setup veröffentlicht das Dashboard auf `http://localhost:8088` und speichert SQLite dauerhaft im Volume `speedtest-data`.

```bash
# Direkt unter Linux/WSL
docker compose up -d --build

# Status und Logs
docker compose ps
docker compose logs --tail=100 speedtest

# Beenden; das Datenvolume bleibt erhalten
docker compose down

# GitHub Container Registry Images
# (automatisch gebaut und veröffentlicht via CI/CD)
docker pull ghcr.io/<your-org>/speedtest:latest
docker run -d --name speedtest -p 8080:8080 -v speedtest-data:/data ghcr.io/<your-org>/speedtest:latest
```

Der Container läuft als nicht privilegierter Benutzer, besitzt keine zusätzlichen Linux-Capabilities und verwendet `/data/speedtest.db`.

## Tech Stack

- **Go** 1.22+ — Backend, Single Binary, `embed.FS`
- **speedtest-go** — Ookla Speedtest Protokoll
- **robfig/cron/v3** — Scheduler
- **modernc.org/sqlite** — Pure-Go SQLite (kein CGO)
- **Vanilla JS + Chart.js** — Frontend (eingebettet)

## Projektstruktur

```
cmd/speedtest/          Entry Point + HTTP Server
internal/engine/        Speedtest-, Bufferbloat-, Traceroute-Engine
internal/scheduler/     Cron-basierter Scheduler
internal/storage/       SQLite (Ergebnisse, Profile, Tarife)
internal/tariff/        Tarif-Domäne: Katalog, Vergleich, Validierung
internal/api/           REST-API + SSE + Broadcaster
web/                    Eingebettetes Frontend (HTML, CSS, JS)
```

## Lizenz

[MIT](LICENSE) — frei verwendbar, unverkäuflich, **ohne Gewährleistung**.

Dieses Projekt nutzt die folgenden Open-Source-Bibliotheken:
- [speedtest-go](https://github.com/showwin/speedtest-go) (MIT)
- [cron/v3](https://github.com/robfig/cron) (MIT)
- [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) (BSD-3-Clause)
- [Chart.js](https://chartjs.org) (MIT)

---

**Haftungsausschluss:** Diese Software wird "wie besehen" ohne jegliche Gewährleistung bereitgestellt. Der Autor haftet nicht für Schäden, die aus der Nutzung entstehen. Speedtest-Messungen sind Näherungswerte und hängen von vielen Faktoren ab (Serverauswahl, Tageszeit, verwendete Hardware). Sie stellen keine verbindliche Bandbreitenmessung im Sinne der Telekom-Richtlinie dar.

Der Mängelmeldung-Generator erstellt ein formales Beschwerdeschreiben auf Basis dieser Näherungswerte. Dies ist **keine Rechtsberatung** — für rechtliche Beratung wenden Sie sich an eine Anwaltskanzlei oder die Verbraucherzentrale (vzbv). Die Nutzung erfolgt auf eigene Verantwortung.
