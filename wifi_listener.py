import serial
import time
import subprocess
import logging
import re

# Set up logging
logging.basicConfig(level=logging.DEBUG,
                   format='[%(levelname)s] %(asctime)s %(message)s',
                   datefmt='%H:%M:%S')

def setup_wifi(ssid, password):
    """Configure WiFi on Orange Pi using nmcli"""
    try:
        # Delete existing connection with same name if it exists
        subprocess.run(['sudo', 'nmcli', 'connection', 'delete', ssid], 
                      stderr=subprocess.DEVNULL)
        
        # Create new wireless connection
        result = subprocess.run([
            'sudo', 'nmcli', 'device', 'wifi', 'connect', ssid,
            'password', password
        ], capture_output=True, text=True)
        
        if result.returncode == 0:
            logging.info(f"Successfully connected to {ssid}")
            return True
        else:
            logging.error(f"Failed to connect: {result.stderr}")
            return False
    except Exception as e:
        logging.error(f"Error setting up WiFi: {e}")
        return False

def get_serial_connection():
    """Create and return a serial connection, with retries"""
    while True:
        try:
            ser = serial.Serial(
                port='/dev/ttyACM0',
                baudrate=115200,
                timeout=1
            )
            logging.info("Serial port opened successfully")
            return ser
        except serial.SerialException as e:
            logging.error(f"Failed to open serial port: {e}")
            logging.info("Retrying in 2 seconds...")
            time.sleep(2)

def main():
    ser = None
    
    while True:
        try:
            if ser is None:
                logging.info("Starting WiFi credential listener...")
                ser = get_serial_connection()
                logging.info("Waiting for ESP32 credentials...")

            if ser.in_waiting:
                line = ser.readline().decode('utf-8', errors='replace').strip()
                if line:
                    logging.debug(f"Received raw data: {line}")
                    
                    # Look for the provisioned credentials format
                    if "Provisioned Credentials: SSID:" in line:
                        try:
                            # Extract SSID and password using regex
                            match = re.search(r"SSID: ([^,]+), Password: ([^\s]+)", line)
                            if match:
                                ssid = match.group(1)
                                password = match.group(2)
                                logging.info(f"Found credentials for network: {ssid}")
                                
                                if setup_wifi(ssid, password):
                                    logging.info("WiFi configuration successful")
                                else:
                                    logging.error("Failed to configure WiFi")
                        except Exception as e:
                            logging.error(f"Error processing credentials: {e}")
            
            time.sleep(0.1)  # Prevent CPU hogging
            
        except (serial.SerialException, OSError) as e:
            logging.error(f"Serial connection lost: {e}")
            if ser:
                try:
                    ser.close()
                except:
                    pass
                ser = None
            time.sleep(2)  # Wait before trying to reconnect
            continue
            
        except KeyboardInterrupt:
            logging.info("Shutting down...")
            break
            
    if ser and ser.is_open:
        ser.close()

if __name__ == "__main__":
    main()
