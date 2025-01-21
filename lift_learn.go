package main
import (
    "encoding/json"
    "fmt"
    "io/ioutil"
    "log"
    "os"
    "os/exec"
    "strings"
    "go.bug.st/serial"
)

type VideoMapping struct {
    TagToVideo map[string]string
}

func main() {
    // Set XDG_RUNTIME_DIR if not set
    if os.Getenv("XDG_RUNTIME_DIR") == "" {
        os.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
    }

    // Read mapping file
    data, err := ioutil.ReadFile("tag_video_map.json")
    if err != nil {
        log.Fatal(err)
    }
    
    var mapping VideoMapping
    if err := json.Unmarshal(data, &mapping.TagToVideo); err != nil {
        log.Fatal(err)
    }

    mode := &serial.Mode{
        BaudRate: 9600,
        DataBits: 8,
        Parity:   serial.NoParity,
        StopBits: serial.OneStopBit,
    }

    port, err := serial.Open("/dev/ttyACM0", mode)
    if err != nil {
        log.Fatal(err)
    }
    defer port.Close()

    var currentCmd *exec.Cmd
    buff := make([]byte, 100)

    for {
        n, err := port.Read(buff)
        if err != nil {
            log.Fatal(err)
        }

        if n > 0 {
            data := string(buff[:n])
            if strings.Contains(data, "UID Value:") {
                uid := data[strings.Index(data, "UID Value:")+11:]
                uid = strings.TrimSpace(strings.Split(uid, "\r\n")[0])
                fmt.Printf("Tag UID: %s\n", uid)

                if videoPath, exists := mapping.TagToVideo[uid]; exists {
                    fmt.Printf("Full video path: %s\n", videoPath)
                    
                    // Check if file exists
                    if _, err := os.Stat(videoPath); err != nil {
                        log.Printf("Video file error: %v\n", err)
                        continue
                    }

                    // Kill previous video if it's still running
                    if currentCmd != nil && currentCmd.Process != nil {
                        fmt.Println("Killing previous video")
                        currentCmd.Process.Kill()
                    }

                    fmt.Printf("Playing video: %s\n", videoPath)
                    currentCmd = exec.Command("mpv", 
                        "--msg-level=all=v",  // Added verbose logging
                        "--no-audio",
                        "--fs",
                        "--loop",
                        videoPath)

                    // Print the full command being executed
                    fmt.Printf("Running command: mpv %s\n", strings.Join(currentCmd.Args[1:], " "))

                    // Capture and display any error output
                    currentCmd.Stderr = os.Stderr
                    currentCmd.Stdout = os.Stdout

                    // Start the command without waiting for it to complete
                    err := currentCmd.Start()
                    if err != nil {
                        log.Printf("Error starting video: %v\n", err)
                    } else {
                        log.Printf("MPV started successfully\n")
                        // Add error checking on the process
                        go func() {
                            err := currentCmd.Wait()
                            if err != nil {
                                log.Printf("MPV process error: %v\n", err)
                            }
                        }()
                    }
                }
            }
        }
    }
}
