package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"
	"encoding/json"
    "math/rand"
    "net/http"
    "strings"


	"firecracker-manager/internal/microvm"
)

func main() {
	var (
		action = flag.String("action", "test", "Action: create, destroy, test, or server")
		vmID   = flag.String("id", "test-vm", "MicroVM ID")
		image  = flag.String("image", "alpine:latest", "Container image to run")
		port   = flag.String("port", "8080", "Server port")
	)
	flag.Parse()

	ctx := context.Background()

	// Skip manager creation for server mode
	if *action == "server" {
		if err := startServer(*port); err != nil {
			log.Fatalf("Server failed: %v", err)
		}
		return
	}

	dataDir := "/tmp/firecracker-data"
	manager, err := microvm.NewManager(dataDir)
	if err != nil {
		log.Fatalf("Failed to create microVM manager: %v", err)
	}
	defer manager.Close()

	switch *action {
	// case "create":
	// 	err = createVM(ctx, manager, *vmID, *image)
	// case "destroy":
	// 	err = destroyVM(ctx, manager, *vmID)
	case "test":
		err = testLifecycle(ctx, manager, *vmID, *image)
	default:
		fmt.Printf("Usage: %s -action=[create|destroy|test|server] -id=<vm-id> -image=<image>\n", os.Args[0])
		os.Exit(1)
	}

	if err != nil {
		log.Fatalf("Operation failed: %v", err)
	}
}

func createVM(ctx context.Context, manager *microvm.Manager, vmID, image string) error {
	fmt.Printf("Creating microVM %s with image %s\n", vmID, image)

	err := manager.Create(ctx, vmID, image)
	if err != nil {
		return fmt.Errorf("failed to create microVM: %w", err)
	}

	fmt.Printf("MicroVM %s created successfully\n", vmID)
	return nil
}

func testLifecycle(ctx context.Context, manager *microvm.Manager, vmID, image string) error {
	fmt.Printf("Testing microVM lifecycle with %s\n", image)

	// Create
	err := manager.Create(ctx, vmID, image)
	if err != nil {
		return fmt.Errorf("failed to create microVM: %w", err)
	}
	fmt.Printf("✓ Created microVM %s\n", vmID)

	// Wait a bit
	fmt.Println("Waiting 5 seconds...")
	time.Sleep(5 * time.Second)

	// Check status
	vms, err := manager.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list VMs: %w", err)
	}

	for _, vm := range vms {
		if vm.ID == vmID {
			fmt.Printf("✓ Status: %s\n", vm.Status)
			break
		}
	}

	// Destroy
	err = manager.Destroy(ctx, vmID)
	if err != nil {
		return fmt.Errorf("failed to destroy microVM: %w", err)
	}
	fmt.Printf("✓ Destroyed microVM %s\n", vmID)

	fmt.Println("Test completed successfully!")
	return nil
}

func startServer(port string) error {
    mux := http.NewServeMux()

    // Fake data
    fakePackages := map[string]interface{}{
        "available": []map[string]string{
            {"name": "ros2-navigation", "version": "2.1.0", "description": "Nav2 stack"},
            {"name": "ros2-slam", "version": "1.5.2", "description": "SLAM toolbox"},
            {"name": "ros2-perception", "version": "3.0.1", "description": "Perception pipeline"},
        },
        "installed": []map[string]string{
            {"name": "ros2-core", "version": "2.0.0", "status": "active"},
            {"name": "ros2-drivers", "version": "1.8.0", "status": "active"},
        },
    }

    fakeNodes := map[string]interface{}{
        "running": []map[string]interface{}{
            {"name": "camera_node", "status": "active", "pid": 1234, "cpu": 15.2, "memory": 128},
            {"name": "lidar_node", "status": "active", "pid": 1235, "cpu": 8.5, "memory": 64},
        },
        "stopped": []map[string]interface{}{
            {"name": "slam_node", "status": "inactive", "pid": 0},
        },
    }

    // Package endpoints
    mux.HandleFunc("/api/v1/packages", func(w http.ResponseWriter, r *http.Request) {
        log.Printf("%s /api/v1/packages", r.Method)
        json.NewEncoder(w).Encode(fakePackages)
    })

    mux.HandleFunc("/api/v1/packages/", func(w http.ResponseWriter, r *http.Request) {
        parts := strings.Split(r.URL.Path, "/")
        if len(parts) < 5 {
            http.Error(w, "invalid path", 400)
            return
        }
        pkgName := parts[4]
        action := ""
        if len(parts) > 5 {
            action = parts[5]
        }

        log.Printf("%s /api/v1/packages/%s/%s", r.Method, pkgName, action)

        if r.Method == "POST" && action == "install" {
            resp := map[string]string{
                "job_id": fmt.Sprintf("job-%d", time.Now().Unix()),
                "status":  "installing",
                "package": pkgName,
            }
            w.WriteHeader(202)
            json.NewEncoder(w).Encode(resp)
            return
        }

        if r.Method == "DELETE" {
            resp := map[string]string{
                "job_id": fmt.Sprintf("job-%d", time.Now().Unix()),
                "status":  "uninstalling",
                "package": pkgName,
            }
            w.WriteHeader(202)
            json.NewEncoder(w).Encode(resp)
            return
        }

        http.Error(w, "not found", 404)
    })

    // Node endpoints
    mux.HandleFunc("/api/v1/nodes", func(w http.ResponseWriter, r *http.Request) {
        log.Printf("%s /api/v1/nodes", r.Method)
        json.NewEncoder(w).Encode(fakeNodes)
    })

    mux.HandleFunc("/api/v1/nodes/", func(w http.ResponseWriter, r *http.Request) {
        parts := strings.Split(r.URL.Path, "/")
        if len(parts) < 5 {
            http.Error(w, "invalid path", 400)
            return
        }
        nodeName := parts[4]
        action := ""
        if len(parts) > 5 {
            action = parts[5]
        }

        log.Printf("%s /api/v1/nodes/%s/%s", r.Method, nodeName, action)

        if action == "start" && r.Method == "POST" {
            resp := map[string]interface{}{
                "status": "active",
                "pid":    rand.Intn(9000) + 1000,
                "node":   nodeName,
            }
            json.NewEncoder(w).Encode(resp)
            return
        }

        if action == "stop" && r.Method == "POST" {
            resp := map[string]interface{}{
                "status": "inactive",
                "node":   nodeName,
            }
            json.NewEncoder(w).Encode(resp)
            return
        }

        if action == "status" && r.Method == "GET" {
            resp := map[string]interface{}{
                "status": "active",
                "cpu":    rand.Float64() * 20,
                "memory": rand.Intn(200) + 50,
                "uptime": rand.Intn(86400),
            }
            json.NewEncoder(w).Encode(resp)
            return
        }

        if action == "logs" && r.Method == "GET" {
            lines := r.URL.Query().Get("lines")
            resp := map[string]interface{}{
                "logs": []map[string]string{
                    {"timestamp": time.Now().Add(-5 * time.Minute).Format(time.RFC3339), "level": "INFO", "message": "Node started"},
                    {"timestamp": time.Now().Add(-3 * time.Minute).Format(time.RFC3339), "level": "INFO", "message": "Processing data"},
                    {"timestamp": time.Now().Add(-1 * time.Minute).Format(time.RFC3339), "level": "WARN", "message": "High CPU usage"},
                },
                "total_lines": 1500,
                "requested":   lines,
            }
            json.NewEncoder(w).Encode(resp)
            return
        }

        http.Error(w, "not found", 404)
    })

    // Job status
    mux.HandleFunc("/api/v1/jobs/", func(w http.ResponseWriter, r *http.Request) {
        parts := strings.Split(r.URL.Path, "/")
        if len(parts) < 5 {
            http.Error(w, "invalid path", 400)
            return
        }
        jobID := parts[4]

        log.Printf("%s /api/v1/jobs/%s", r.Method, jobID)

        resp := map[string]interface{}{
            "job_id": jobID,
            "status": "completed",
            "result": "success",
            "logs":   "Package installed successfully\nConfiguring dependencies\nDone",
        }
        json.NewEncoder(w).Encode(resp)
    })

    // Health check
    mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
    })

    addr := ":" + port
    fmt.Printf("Demo API server running on http://localhost%s\n", addr)
    fmt.Println("Endpoints:")
    fmt.Println("  GET  /api/v1/packages")
    fmt.Println("  POST /api/v1/packages/{name}/install")
    fmt.Println("  GET  /api/v1/nodes")
    fmt.Println("  POST /api/v1/nodes/{name}/start")
    fmt.Println("  GET  /api/v1/nodes/{name}/status")
    fmt.Println("  GET  /api/v1/jobs/{id}")

    return http.ListenAndServe(addr, mux)
}
