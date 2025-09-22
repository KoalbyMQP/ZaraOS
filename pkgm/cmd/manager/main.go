package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"firecracker-manager/internal/healthcheck"
	"firecracker-manager/internal/microvm"
)

func main() {
	var (
		action = flag.String("action", "test", "Action: create, destroy, list, test, or healthcheck")
		vmID   = flag.String("id", "test-vm", "MicroVM ID")
		image  = flag.String("image", "alpine:latest", "Container image to run")
	)
	flag.Parse()

	ctx := context.Background()
	dataDir := "/tmp/firecracker-data"

	// Handle healthcheck without needing manager
	if *action == "healthcheck" {
		if err := healthcheck.RunHealthCheck(dataDir); err != nil {
			os.Exit(1)
		}
		return
	}

	manager, err := microvm.NewManager(dataDir)
	if err != nil {
		log.Fatalf("Failed to create microVM manager: %v", err)
	}
	defer manager.Close()

	switch *action {
	case "create":
		err = createVM(ctx, manager, *vmID, *image)
	case "destroy":
		err = destroyVM(ctx, manager, *vmID)
	case "list":
		err = listVMs(ctx, manager)
	case "test":
		err = testLifecycle(ctx, manager, *vmID, *image)
	default:
		fmt.Printf("Usage: %s -action=[create|destroy|list|test|healthcheck] -id=<vm-id> -image=<image>\n", os.Args[0])
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

func destroyVM(ctx context.Context, manager *microvm.Manager, vmID string) error {
	fmt.Printf("Destroying microVM %s\n", vmID)

	err := manager.Destroy(ctx, vmID)
	if err != nil {
		return fmt.Errorf("failed to destroy microVM: %w", err)
	}

	fmt.Printf("MicroVM %s destroyed successfully\n", vmID)
	return nil
}

func listVMs(ctx context.Context, manager *microvm.Manager) error {
	fmt.Println("Listing microVMs...")

	vms, err := manager.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list microVMs: %w", err)
	}

	if len(vms) == 0 {
		fmt.Println("No microVMs found")
		return nil
	}

	fmt.Printf("%-20s %s\n", "VM ID", "STATUS")
	fmt.Println(strings.Repeat("-", 35))
	for _, vm := range vms {
		fmt.Printf("%-20s %s\n", vm.ID, vm.Status)
	}

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
		return fmt.Errorf("failed to get status: %w", err)
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