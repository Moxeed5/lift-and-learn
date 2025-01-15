package main

import (
   "bytes"
   "encoding/json"
   "fmt"
   "io"
   "log"
   "net"
   "net/http"
   "os"
   "path/filepath"
   "sync"
   "time"
)

const (
   AWS_REGISTRY_ENDPOINT = "https://on9p48hjz3.execute-api.us-east-2.amazonaws.com/default/RegisterDevice"
   DEVICE_ID            = "OP5-MAX-TEST-001"  // Changed to be more identifiable
   STORAGE_PATH         = "./content"    
   HEARTBEAT_INTERVAL   = 60 * time.Second
)

type UploadRequest struct {
   DeploymentId string  `json:"deploymentId"`
   ProjectId    string  `json:"projectId"`
   CustomerId   string  `json:"customerId"`
   Things       []Thing `json:"things"`
}

type Thing struct {
   ProductId   string `json:"productId"`
   MediaUrl    string `json:"mediaUrl"`
   NfcTagId    string `json:"nfcTagId"`
   ProductName string `json:"productName"`
}

type DeviceRegistration struct {
   DeviceId  string `json:"deviceId"`
   IpAddress string `json:"ipAddress"`
   Status    string `json:"status"`
}

func getLocalIP() (string, error) {
   log.Println("Attempting to get local IP address...")
   
   ifaces, err := net.Interfaces()
   if err != nil {
       return "", fmt.Errorf("failed to get network interfaces: %v", err)
   }
   
   for _, iface := range ifaces {
       log.Printf("Checking interface: %s", iface.Name)
       
       if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
           log.Printf("Skipping interface %s (loopback or down)", iface.Name)
           continue
       }

       addrs, err := iface.Addrs()
       if err != nil {
           log.Printf("Failed to get addresses for interface %s: %v", iface.Name, err)
           continue
       }

       for _, addr := range addrs {
           if ipnet, ok := addr.(*net.IPNet); ok {
               if ipnet.IP.IsLoopback() || ipnet.IP.To4() == nil {
                   log.Printf("Skipping IP %s (loopback or IPv6)", ipnet.IP)
                   continue
               }
               log.Printf("Found valid IP: %s", ipnet.IP.String())
               return ipnet.IP.String(), nil
           }
       }
   }
   
   return "", fmt.Errorf("no valid IP address found")
}

func registerWithAWS(ip string) error {
   log.Printf("Attempting to register device %s with IP %s", DEVICE_ID, ip)
   
   registration := DeviceRegistration{
       DeviceId:  DEVICE_ID,
       IpAddress: ip,
       Status:    "online",
   }

   jsonData, err := json.Marshal(registration)
   if err != nil {
       return fmt.Errorf("failed to marshal registration data: %v", err)
   }
   
   log.Printf("Sending registration request to AWS: %s", string(jsonData))

   resp, err := http.Post(AWS_REGISTRY_ENDPOINT, "application/json", bytes.NewBuffer(jsonData))
   if err != nil {
       return fmt.Errorf("failed to send registration request: %v", err)
   }
   defer resp.Body.Close()

   // Read and log response body
   body, err := io.ReadAll(resp.Body)
   if err != nil {
       return fmt.Errorf("failed to read response body: %v", err)
   }
   log.Printf("AWS Response Status: %d, Body: %s", resp.StatusCode, string(body))

   if resp.StatusCode != http.StatusOK {
       return fmt.Errorf("failed to register device: status=%d body=%s", resp.StatusCode, string(body))
   }

   log.Printf("Successfully registered device %s with AWS", DEVICE_ID)
   return nil
}

func startHeartbeat() {
   ticker := time.NewTicker(HEARTBEAT_INTERVAL)
   go func() {
       for range ticker.C {
           ip, err := getLocalIP()
           if err != nil {
               log.Printf("Failed to get IP: %v", err)
               continue
           }
           
           if err := registerWithAWS(ip); err != nil {
               log.Printf("Heartbeat failed: %v", err)
           }
       }
   }()
}

func handleUpload(w http.ResponseWriter, r *http.Request) {
   if r.Method != http.MethodPost {
       http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
       return
   }

   var req UploadRequest
   if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
       http.Error(w, "Invalid request body", http.StatusBadRequest)
       return
   }

   // Create project directory
   projectDir := filepath.Join(STORAGE_PATH, req.ProjectId)
   if err := os.MkdirAll(projectDir, 0755); err != nil {
       http.Error(w, "Failed to create project directory", http.StatusInternalServerError)
       return
   }

   // Process each thing concurrently
   var wg sync.WaitGroup
   errorsChan := make(chan error, len(req.Things))

   for _, thing := range req.Things {
       wg.Add(1)
       go func(t Thing) {
           defer wg.Done()
           if err := processContent(projectDir, t); err != nil {
               errorsChan <- fmt.Errorf("failed to process %s: %v", t.ProductId, err)
           }
       }(thing)
   }

   // Wait for all goroutines to finish
   wg.Wait()
   close(errorsChan)

   // Check for any errors
   var errors []string
   for err := range errorsChan {
       errors = append(errors, err.Error())
   }

   if len(errors) > 0 {
       response := map[string]interface{}{
           "status": "partial_success",
           "errors": errors,
       }
       json.NewEncoder(w).Encode(response)
       return
   }

   json.NewEncoder(w).Encode(map[string]string{
       "status":  "success",
       "message": fmt.Sprintf("Successfully processed deployment %s", req.DeploymentId),
   })
}

func processContent(projectDir string, thing Thing) error {
   // Get the file from the MediaUrl
   resp, err := http.Get(thing.MediaUrl)
   if err != nil {
       return fmt.Errorf("failed to download content: %v", err)
   }
   defer resp.Body.Close()

   if resp.StatusCode != http.StatusOK {
       return fmt.Errorf("failed to download content, status: %d", resp.StatusCode)
   }

   // Create file path
   filename := filepath.Join(projectDir, fmt.Sprintf("%s.mp4", thing.ProductId))
   
   // Create the file
   out, err := os.Create(filename)
   if err != nil {
       return fmt.Errorf("failed to create file: %v", err)
   }
   defer out.Close()

   // Copy the content
   if _, err := io.Copy(out, resp.Body); err != nil {
       return fmt.Errorf("failed to save content: %v", err)
   }

   // Save metadata
   metadataFilename := filepath.Join(projectDir, fmt.Sprintf("%s.json", thing.ProductId))
   metadataFile, err := os.Create(metadataFilename)
   if err != nil {
       return fmt.Errorf("failed to create metadata file: %v", err)
   }
   defer metadataFile.Close()

   return json.NewEncoder(metadataFile).Encode(thing)
}

func startServer() {
   // Ensure storage directory exists
   if err := os.MkdirAll(STORAGE_PATH, 0755); err != nil {
       log.Fatal(err)
   }

   mux := http.NewServeMux()
   mux.HandleFunc("/receive-content", handleUpload)

   server := &http.Server{
       Addr:    ":3000",
       Handler: mux,
   }

   log.Printf("Starting upload server on port 3000")
   if err := server.ListenAndServe(); err != nil {
       log.Fatal(err)
   }
}

func main() {
   // Register device with AWS on startup
   ip, err := getLocalIP()
   if err != nil {
       log.Fatal(err)
   }
   
   if err := registerWithAWS(ip); err != nil {
       log.Fatal(err)
   }

   // Start heartbeat
   startHeartbeat()

   // Start server
   go startServer()

   // Keep main thread alive
   select {}
}
