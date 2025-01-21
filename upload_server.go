package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

const (
	AWS_REGISTRY_ENDPOINT = "https://on9p48hjz3.execute-api.us-east-2.amazonaws.com/default/RegisterDevice"
	DEVICE_ID             = "OP5-MAX-TEST-001"
	STORAGE_PATH          = "./content"
)

// Device registration structure
type DeviceRegistration struct {
	DeviceId  string `json:"deviceId"`
	IpAddress string `json:"ipAddress"`
}

// Upload request structure
type UploadRequest struct {
	DeploymentId string  `json:"deploymentId"`
	ProjectId    string  `json:"projectId"`
	CustomerId   string  `json:"customerId"`
	Things       []Thing `json:"things"`
}

// Thing structure within UploadRequest
type Thing struct {
	ProductId   string `json:"productId"`
	MediaUrl    string `json:"mediaUrl"`
	NfcTagId    string `json:"nfcTagId"`
	ProductName string `json:"productName"`
}

// Function to fetch the public ngrok URL
func getNgrokURL() (string, error) {
	resp, err := http.Get("http://localhost:4040/api/tunnels")
	if err != nil {
		return "", fmt.Errorf("failed to fetch ngrok URL: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse ngrok response: %v", err)
	}

	tunnels, ok := result["tunnels"].([]interface{})
	if !ok || len(tunnels) == 0 {
		return "", fmt.Errorf("no tunnels found in ngrok response")
	}

	publicURL, ok := tunnels[0].(map[string]interface{})["public_url"].(string)
	if !ok {
		return "", fmt.Errorf("failed to extract public URL from ngrok response")
	}

	log.Printf("Ngrok URL: %s", publicURL)
	return publicURL, nil
}

// Function to register the device with AWS
func registerWithAWS(publicUrl string) error {
	log.Printf("Registering device %s with URL %s", DEVICE_ID, publicUrl)

	registration := DeviceRegistration{
		DeviceId:  DEVICE_ID,
		IpAddress: publicUrl,
	}

	jsonData, err := json.Marshal(registration)
	if err != nil {
		return fmt.Errorf("failed to marshal registration data: %v", err)
	}

	log.Printf("Payload for registration: %s", string(jsonData))

	client := &http.Client{
		Timeout: 30 * time.Second, // Increased timeout for network reliability
	}
	resp, err := client.Post(AWS_REGISTRY_ENDPOINT, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send registration request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("Response from AWS: %s", string(body))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to register device: status=%d body=%s", resp.StatusCode, string(body))
	}

	log.Printf("Successfully registered device %s", DEVICE_ID)
	return nil
}

// Function to handle incoming upload requests
func handleUpload(w http.ResponseWriter, r *http.Request) {
	log.Printf("============ NEW UPLOAD REQUEST ============")
	log.Printf("Received upload request from: %s", r.RemoteAddr)

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req UploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("Error decoding JSON: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	log.Printf("Decoded request: %+v", req)

	projectDir := filepath.Join(STORAGE_PATH, req.ProjectId)
	log.Printf("Creating project directory: %s", projectDir)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		log.Printf("Failed to create project directory: %v", err)
		http.Error(w, "Failed to create project directory", http.StatusInternalServerError)
		return
	}

	var wg sync.WaitGroup
	errorsChan := make(chan error, len(req.Things))

	for _, thing := range req.Things {
		wg.Add(1)
		go func(t Thing) {
			defer wg.Done()
			log.Printf("Processing thing: %+v", t)
			if err := processContent(projectDir, t); err != nil {
				log.Printf("Error processing thing %s: %v", t.ProductId, err)
				errorsChan <- fmt.Errorf("failed to process %s: %v", t.ProductId, err)
			} else {
				log.Printf("Successfully processed thing: %s", t.ProductId)
			}
		}(thing)
	}

	wg.Wait()
	close(errorsChan)

	var errors []string
	for err := range errorsChan {
		errors = append(errors, err.Error())
	}

	if len(errors) > 0 {
		log.Printf("Processing completed with errors: %v", errors)
		response := map[string]interface{}{
			"status": "partial_success",
			"errors": errors,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	log.Printf("All content processed successfully")
	response := map[string]string{
		"status":  "success",
		"message": fmt.Sprintf("Successfully processed deployment %s", req.DeploymentId),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Function to download and store content
func processContent(projectDir string, thing Thing) error {
	log.Printf("Downloading content from: %s", thing.MediaUrl)

	resp, err := http.Get(thing.MediaUrl)
	if err != nil {
		return fmt.Errorf("failed to download content: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download content, status: %d", resp.StatusCode)
	}

	filename := filepath.Join(projectDir, fmt.Sprintf("%s.mp4", thing.ProductId))
	out, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %v", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("failed to save content: %v", err)
	}

	metadataFilename := filepath.Join(projectDir, fmt.Sprintf("%s.json", thing.ProductId))
	metadataFile, err := os.Create(metadataFilename)
	if err != nil {
		return fmt.Errorf("failed to create metadata file: %v", err)
	}
	defer metadataFile.Close()

	if err := json.NewEncoder(metadataFile).Encode(thing); err != nil {
		return fmt.Errorf("failed to save metadata: %v", err)
	}

	log.Printf("Successfully saved content and metadata for product %s", thing.ProductId)
	return nil
}

// Start the server and registration process
func main() {
	go func() {
		cmd := exec.Command("ngrok", "http", "3000")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Run()
	}()

	time.Sleep(5 * time.Second) // Wait for ngrok to start
	ngrokURL, err := getNgrokURL()
	if err != nil {
		log.Fatalf("Error fetching ngrok URL: %v", err)
	}

	if err := registerWithAWS(ngrokURL); err != nil {
		log.Fatalf("Device registration failed: %v", err)
	}

	startServer()
}

func startServer() {
	if err := os.MkdirAll(STORAGE_PATH, 0755); err != nil {
		log.Fatalf("Failed to create storage directory: %v", err)
	}

	http.HandleFunc("/receive-content", handleUpload)

	log.Printf("Starting upload server on port 3000")
	if err := http.ListenAndServe(":3000", nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
