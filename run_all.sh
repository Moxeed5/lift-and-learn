#!/bin/bash
echo "Starting ngrok for HTTP tunneling..."
nohup ngrok http 3000 --config /home/max/.config/ngrok/ngrok.yml > ngrok.log 2>&1 &
sleep 5  # Give ngrok time to initialize

echo "Starting WiFi credential listener..."
sudo python3 wifi_listener.py

echo "Registering Device...."
go run upload_server.go

echo "Starting Lift Learn application..."
sudo go run lift_learn.go
