<div align="left">

![GitHub Actions](https://img.shields.io/badge/github%20actions-%232671E5.svg?style=for-the-badge&logo=githubactions&logoColor=white)
[![CI](https://github.com/docfrench/health_checker/actions/workflows/ci.yml/badge.svg)](https://github.com/docfrench/health_checker/actions/workflows/ci.yml)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/docfrench/health_checker)

</div>

# Health Checker



A small Go service that monitors the health of Docker containers and HTTP endpoints on a single host, logs state changes, and provides a terminal UI for live status. Built for my Unraid server, but it runs anywhere with a Docker socket.

![Live status view](images/live.png)

## Features

- Concurrent polling of containers and HTTP health endpoints (one goroutine per target)
- Persistent log of status changes
- Interactive TUI built with [Bubble Tea](https://github.com/charmbracelet/bubbletea): live status view and log tail
- Targets defined in a single `config.json`


## Quick start

```bash
cp config.json.example config.json
# edit config.json with your server name and targets

docker run -d --name health_checker \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -v $(pwd)/config.json:/app/config.json:ro \
  -v $(pwd)/data:/app/data \
  health_checker   # TODO: published image name, or `docker build -t health_checker .`
```

Open the TUI:

```bash
docker exec -it health_checker ./health_checker --tui
```

## Configuration

```json
{
    "server_name": "My Server",    
    "containers": [
    {"label":"NPM","name":"NginxProxyManager"}
    // label = what the TUI shows to you; name = container name
    ],
    "http_endpoints": [
    {"label": "My Website", "url": "https://mywebsite.com/health"}
    ]
}
```



## How it works

On startup it loads `config.json` and spawns a goroutine per target. Each one polls on an interval, either asking the Docker daemon for container state or issuing an HTTP GET against the endpoint. Results are written to the log and to `status.json`, which the TUI reads. 

```json
{
  "updated_at": "2026-09-13T14:32:01-04:00",
  "containers": [
    {"label": "NPM", "status": "running", "health": "healthy", "checked_at": "2026-09-13T14:32:00-04:00"},
    {"label": "Jellyfin", "status": "running", "health": "n/a", "checked_at": "2026-09-13T14:32:00-04:00"}
  ],
  "http_checks": [
    {"label": "Deimos Archive FastAPI", "status_code": 200, "checked_at": "2026-09-13T14:31:58-04:00"}
  ]
}
```

## Views

| Logs | Live status |
|------|-------------|
| ![Logs](images/logs.png) | ![Live](images/live.png) |

## Development

```bash
go build -o health_checker .
go test ./...    # TODO: if tests exist; CI runs: <what ci.yml does>
```

Requires Go 1.26

## License

See LICENSE file
