package main

   import (
      "fmt"
      "net/http"
      "os"
      "sync"
      "time"
      "os/exec"
   )

   const apiURL = "https://deimosarchive.com/health"
   const NPMhealth = "NPM.sh"
   const interval = 7400 * time.Second

   func main() {
      var wg sync.WaitGroup
      // Open file once at the start
      file, err := os.OpenFile("health_checker.syslog", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
      if err != nil {
         fmt.Println("Error opening file:", err)
         return
      }
      defer file.Close()

      wg.Add(2)
      go func() {
         defer wg.Done()
         for {
            resp, err := http.Get(apiURL)
            if err != nil {
               result := fmt.Sprintf("Error making request: %v\n", err)
               fmt.Print(result)
               time.Sleep(interval)
               continue
            }
            defer resp.Body.Close()

            now := time.Now()
            result := fmt.Sprintf("[Deimos Archive FastAPI] Status: %d <> Time: %s\n", resp.StatusCode, now.Format("2 Jan 06 03:04PM"))
            fmt.Print(result)

            // Write to file
            _, err = file.WriteString(result)
            if err != nil {
               fmt.Println("Error writing to file:", err)
               return
            }

            time.Sleep(interval)
         }
      }()

      go func() {
         defer wg.Done()
         for {
            cmd := exec.Command("/bin/bash", NPMhealth)
            output, err := cmd.Output()
            if err != nil {
               result := fmt.Sprintf("Error executing script: %v\n", err)
               fmt.Print(result)
               time.Sleep(interval)
               continue
            }
            
            now := time.Now()
            result := fmt.Sprintf("[NPM] Status: %s <> Time: %s\n", string(output), now.Format("2 Jan 06 03:04PM"))
            fmt.Print(result)

            // Write to file
            _, err = file.WriteString(result)
            if err != nil {
               fmt.Println("Error writing to file:", err)
               return
            }

            time.Sleep(interval)
         }
      }()

      wg.Wait()
   }
