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

const apiURL = "https://deimosarchive.com/health"
const npmContainerName = "NginxProxyManager"
const nginxContainer = "nginx-homepage"  
const interval = 7200 * time.Second            

func main() {
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

	wg.Add(3)

	// Goroutine 1: HTTP health check
	go func() {
		defer wg.Done()
		for {
			resp, err := httpClient.Get(apiURL)
			if err != nil {
				result := fmt.Sprintf("Error making request: %v\n", err)
				fmt.Print(result)
				logResult(file, result)
				time.Sleep(interval)
				continue
			}

			now := time.Now()
			result := fmt.Sprintf("[Deimos Archive FastAPI] Status: %d <> Time: %s\n", resp.StatusCode, now.Format("2 Jan 06 03:04PM"))
			fmt.Print(result)
			logResult(file, result)

			// Close explicitly — a defer here never fires since this func never returns
			if err := resp.Body.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "Error closing response body: %v\n", err)
			}

			time.Sleep(interval)
		}
	}()

	// Goroutine 2: Docker container health check via SDK
	go func() {
		defer wg.Done()

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


		for {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			inspect, err := cli.ContainerInspect(ctx, npmContainerName)
			cancel()

			now := time.Now()
			var result string
			if err != nil {
				result = fmt.Sprintf("[NPM] Error inspecting container: %v <> Time: %s\n", err, now.Format("2 Jan 06 03:04PM"))
			} else {
				status := inspect.State.Status // "running", "exited", etc.
				health := "n/a"
				if inspect.State.Health != nil {
					health = inspect.State.Health.Status // "healthy", "unhealthy", "starting"
				}
				result = fmt.Sprintf("[NPM] Status: %s <> Health: %s <> Time: %s\n", status, health, now.Format("2 Jan 06 03:04PM"))
			}
			fmt.Print(result)
			logResult(file, result)

			time.Sleep(interval)
		}
	}()
	go func() {
		defer wg.Done()

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


		for {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			inspect, err := cli.ContainerInspect(ctx, nginxContainer)
			cancel()

			now := time.Now()
			var result string
			if err != nil {
				result = fmt.Sprintf("[NPM] Error inspecting container: %v <> Time: %s\n", err, now.Format("2 Jan 06 03:04PM"))
			} else {
				status := inspect.State.Status // "running", "exited", etc.
				health := "n/a"
				if inspect.State.Health != nil {
					health = inspect.State.Health.Status // "healthy", "unhealthy", "starting"
				}
				result = fmt.Sprintf("[NPM] Status: %s <> Health: %s <> Time: %s\n", status, health, now.Format("2 Jan 06 03:04PM"))
			}
			fmt.Print(result)
			logResult(file, result)

			time.Sleep(interval)
		}
	}()
	wg.Wait()
}

func logResult(file *os.File, result string) {
	if _, err := file.WriteString(result); err != nil {
		fmt.Println("Error writing to file:", err)
	}
}
