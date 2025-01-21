package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
)

type Thing struct {
	ProductId   string `json:"productId"`
	MediaUrl    string `json:"mediaUrl"`
	NfcTagId    string `json:"nfcTagId"`
	ProductName string `json:"productName"`
}

func main() {
	const contentDir = "./content"

	log.Printf("Starting JSON correction in directory: %s", contentDir)

	err := filepath.Walk(contentDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("failed to access path %s: %v", path, err)
		}

		if !info.IsDir() && filepath.Ext(path) == ".json" {
			log.Printf("Processing JSON file: %s", path)
			if err := fixJsonFile(path); err != nil {
				log.Printf("Error fixing JSON file %s: %v", path, err)
			} else {
				log.Printf("Successfully updated JSON file: %s", path)
			}
		}
		return nil
	})

	if err != nil {
		log.Fatalf("Error traversing content directory: %v", err)
	}

	log.Println("JSON correction completed successfully.")
}

func fixJsonFile(filePath string) error {
	// Read the JSON file
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read JSON file: %v", err)
	}

	var thing Thing
	if err := json.Unmarshal(data, &thing); err != nil {
		return fmt.Errorf("failed to parse JSON: %v", err)
	}

	// Update the `mediaUrl` to point to the local file
	dir := filepath.Dir(filePath)
	thing.MediaUrl = filepath.Join(dir, fmt.Sprintf("%s.mp4", thing.ProductId))

	// Write the updated JSON back to the file
	updatedData, err := json.MarshalIndent(thing, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal updated JSON: %v", err)
	}

	if err := ioutil.WriteFile(filePath, updatedData, 0644); err != nil {
		return fmt.Errorf("failed to write updated JSON file: %v", err)
	}

	return nil
}
