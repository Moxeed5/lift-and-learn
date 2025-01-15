#!/bin/bash
echo "Starting WiFi credential listener..."
sudo python3 wifi_listener.py
echo "Registering Device...."
go run upload_server.go
echo "Starting Lift Learn application..."
sudo go run lift_learn.go
