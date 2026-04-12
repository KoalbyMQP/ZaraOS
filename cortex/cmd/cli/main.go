// cortex CLI — dev tool for building and running packages against the local Cortex server.
// Talks to 127.0.0.1:8080 with no auth (localhost bypass).
// Never shipped to ZaraOS or the Pi.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	cortexAddr  = "http://127.0.0.1:8080"
	lastIDFile  = ".cortex-last"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "build":
		err = cmdBuild()
	case "run":
		err = cmdRun(os.Args[2:])
	case "logs":
		err = cmdLogs(os.Args[2:])
	case "stop":
		err = cmdStop()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		usage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// cmdBuild runs: nerdctl build -t <package-name>:dev .
func cmdBuild() error {
	name, err := packageName()
	if err != nil {
		return err
	}
	tag := name + ":dev"
	fmt.Printf("building %s\n", tag)
	cmd := exec.Command("nerdctl", "build", "-t", tag, ".")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// cmdRun posts to POST /instances, optionally with a local image override.
// Writes the returned instance ID to ~/.cortex-last.
func cmdRun(args []string) error {
	var image string
	for i := 0; i < len(args); i++ {
		if args[i] == "--image" && i+1 < len(args) {
			image = args[i+1]
			i++
		}
	}

	name, err := packageName()
	if err != nil {
		return err
	}

	body := map[string]any{"app": name}
	if image != "" {
		body["image"] = image
	}

	data, _ := json.Marshal(body)
	resp, err := http.Post(cortexAddr+"/instances", "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("POST /instances: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(raw))
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &result); err != nil || result.ID == "" {
		return fmt.Errorf("unexpected response: %s", string(raw))
	}

	if err := writeLastID(result.ID); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not write %s: %v\n", lastIDFile, err)
	}

	fmt.Println(result.ID)
	return nil
}

// cmdLogs streams or fetches logs for the last instance ID.
func cmdLogs(args []string) error {
	follow := false
	for _, a := range args {
		if a == "--follow" || a == "-f" {
			follow = true
		}
	}

	id, err := readLastID()
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/instances/%s/logs", cortexAddr, id)
	if follow {
		url += "?stream=true"
	}

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(raw))
	}

	if follow {
		// SSE stream: parse event/data frames line-by-line.
		scanner := bufio.NewScanner(resp.Body)
		var eventType, dataStr string
		for scanner.Scan() {
			line := scanner.Text()

			switch {
			case strings.HasPrefix(line, "event: "):
				eventType = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				dataStr = strings.TrimPrefix(line, "data: ")
			case line == "":
				// Blank line = end of frame; dispatch the event.
				if dataStr == "" {
					eventType = ""
					continue
				}
				switch eventType {
				case "log":
					var evt struct {
						Stream  string `json:"stream"`
						Content string `json:"content"`
					}
					if err := json.Unmarshal([]byte(dataStr), &evt); err == nil {
						if evt.Stream == "stderr" {
							fmt.Printf("[stderr] %s\n", evt.Content)
						} else {
							fmt.Println(evt.Content)
						}
					} else {
						fmt.Println(dataStr)
					}
				case "heartbeat":
					// silently ignore
				case "error":
					fmt.Fprintf(os.Stderr, "stream: %s\n", dataStr)
				default:
					// Backward compat: bare data without event type.
					fmt.Println(dataStr)
				}
				eventType = ""
				dataStr = ""
			}
		}
		if err := scanner.Err(); err != nil {
			return err
		}
		return nil
	}

	_, err = io.Copy(os.Stdout, resp.Body)
	return err
}

// cmdStop sends DELETE /instances/{id} for the last instance ID.
func cmdStop() error {
	id, err := readLastID()
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/instances/%s", cortexAddr, id)
	req, _ := http.NewRequest(http.MethodDelete, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("DELETE %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(raw))
	}

	fmt.Printf("stopped %s\n", id)
	return nil
}

// packageName returns the basename of the current working directory.
func packageName() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("could not determine working directory: %w", err)
	}
	return filepath.Base(cwd), nil
}

func lastIDPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, lastIDFile), nil
}

func readLastID() (string, error) {
	path, err := lastIDPath()
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("no instance ID found — run 'cortex run' first (%w)", err)
	}
	id := strings.TrimSpace(string(raw))
	if id == "" {
		return "", fmt.Errorf("%s is empty — run 'cortex run' first", path)
	}
	return id, nil
}

func writeLastID(id string) error {
	path, err := lastIDPath()
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(id+"\n"), 0644)
}

func usage() {
	fmt.Fprintln(os.Stderr, `cortex — local dev CLI for Cortex

Usage:
  cortex build               Build image: nerdctl build -t <dir>:dev .
  cortex run [--image <ref>] Start instance (optionally with local image)
  cortex logs [--follow]     Stream or fetch logs for last instance
  cortex stop                Stop last instance`)
}
