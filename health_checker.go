package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/docker/docker/client"
)

const (
	apiURL      = "https://deimosarchive.com/health"
	interval    = 7200 * time.Second
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
)

// containerTarget pairs a display label with the actual Docker container name.
type containerTarget struct {
	label string
	name  string
}

var containers = []containerTarget{
	{"NPM", "NginxProxyManager"},
	{"Jellyfin", "Jellyfin"},
	{"LocalStack", "localstack-main"},
	{"FileBrowser", "FileBrowserQuantum"},
	{"Kavita", "kavita"},
	{"AudioBookShelf", "audiobookshelf"},
	{"NGINX", "nginx-homepage"},
	{"Cloudflare-DDNS", "Cloudflare-DDNS"},
	{"AudioBookShelf", "audiobookshelf"},
}

func main() {
	fmt.Printf("%sDeimos Archive Container Status%s\n", colorYellow, colorReset)	
    var wg sync.WaitGroup

	file, err := os.OpenFile("health_checker.syslog", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("Error opening file:", err)

		return
	}
	defer func() {
		if err := file.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing file: %v\n", err)
		}
	}()

	httpClient := &http.Client{Timeout: 10 * time.Second}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		fmt.Printf("Error creating Docker client: %v\n", err)
		return
	}
	defer func() {
		if err := cli.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing Docker client: %v\n", err)
		}
	}()

	// wg count: 1 HTTP checker + 1 goroutine per container target
	wg.Add(1 + len(containers))

	// Goroutine: HTTP health check
	go func() {
        var result string
	    var color string
		defer wg.Done()
		for {
			resp, err := httpClient.Get(apiURL)
			if err != nil {
				result := fmt.Sprintf("Error making request: %v\n", err)
                color = colorRed	
				logResult(file, result)
				continue
			}

			now := time.Now()
			result := fmt.Sprintf("[Deimos Archive FastAPI] Status: %d <> Time: %s\n", resp.StatusCode, now.Format("2 Jan 06 03:04PM"))
            color = colorGreen
			logResult(file, result)

			if err := resp.Body.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "Error closing response body: %v\n", err)
			}

			time.Sleep(interval)
		}
        fmt.Printf("%s%s%s", color, result, colorReset)
        time.Sleep(interval)
	}()

	// One goroutine per container target, all sharing the single Docker client.
	// cli is safe to share across goroutines for reads like ContainerInspect.
	for _, target := range containers {
		target := target // capture loop variable per-iteration
		go func() {
			defer wg.Done()
			for {
				checkContainer(cli, file, target)
				time.Sleep(interval)
			}
		}()
	}

	wg.Wait()
}

func checkContainer(cli *client.Client, file *os.File, target containerTarget) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	inspect, err := cli.ContainerInspect(ctx, target.name)
	cancel()

	now := time.Now()
	var result string
	var color string

	if err != nil {
		result = fmt.Sprintf("[%s] Error inspecting container: %v <> Time: %s\n", target.label, err, now.Format("2 Jan 06 03:04PM"))
		color = colorRed
	} else {
		status := inspect.State.Status // "running", "exited", etc.
		health := "n/a"
		if inspect.State.Health != nil {
			health = inspect.State.Health.Status // "healthy", "unhealthy", "starting"
		}
		result = fmt.Sprintf("[%s] Status: %s <> Health: %s <> Time: %s\n", target.label, status, health, now.Format("2 Jan 06 03:04PM"))
		if status == "running" {
			color = colorGreen
		} else {
			color = colorYellow
		}
	}

	fmt.Printf("%s%s%s", color, result, colorReset)
	logResult(file, result)
}

func logResult(file *os.File, result string) {
	if _, err := file.WriteString(result); err != nil {
		fmt.Println("Error writing to file:", err)
	}
}
