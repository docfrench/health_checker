[![CI](https://github.com/docfrench/health_checker/actions/workflows/ci.yml/badge.svg)](https://github.com/docfrench/health_checker/actions/workflows/ci.yml)

# Health Checker

![alt text](https://github.com/docfrench/health_checker/blob/main/images/main.png "Main screen")

A simple Go binary to track the health status of local Docker containers. Created for deployment on my Unraid server.

Uses concurrent goroutines to poll the status of Docker containers (and HTTP API health endpoints), and writes them to a persistent log. Can also display the live status. Containers / API endpoints are set through a config.json, edited by the user.

Uses [Bubble Tea](https://github.com/charmbracelet/bubbletea/tree/main "Bubble Tea") for a TUI.

>The fun, functional and stateful way to build terminal apps. A Go framework based on The Elm Architecture. Bubble Tea is well-suited for simple and complex terminal applications, either inline, full-window, or a mix of both.


## Use
Deploy as a Docker container on your Docker host. 

Edit config.json.example -> config.json

Add the name of your server and any endpoints you want to monitor. SSH into your Docker host and run:

```
docker exec -it health_checker ./health_checker --tui
```

Once connected, choose to either view the logs:

![alt text](https://github.com/docfrench/health_checker/blob/main/images/logs.png "Tail logs")

Or view live status of containers:

![alt text](https://github.com/docfrench/health_checker/blob/main/images/live.png "Live status")
