package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"firecracker-manager/internal/microvm"
)

func main() {
	var (
		action = flag.String("action", "test", "Action: create, destroy, or test")
		vmID   = flag.String("id", "test-vm", "MicroVM ID")
		image  = flag.String("image", "alpine:latest", "Container image to run")
	)
	flag.Parse()

	ctx := context.Background()

	manager, err := microvm.NewManager()
	if err != nil {
		log.Fatalf("Failed to create microVM manager: %v", err)
	}
	defer manager.Close()

	switch *action {
	case "create":
		err = createVM(ctx, manager, *vmID, *image)
	case "destroy":
		err = destroyVM(ctx, manager, *vmID)
	case "test":
		err = testLifecycle(ctx, manager, *vmID, *image)
	default:
		fmt.Printf("Usage: %s -action=[create|destroy|test] -id=<vm-id> -image=<image>\n", os.Args[0])
		os.Exit(1)
	}

	if err != nil {
		log.Fatalf("Operation failed: %v", err)
	}
}

func createVM(ctx context.Context, manager *microvm.Manager, vmID, image string) error {
	fmt.Printf("Creating microVM %s with image %s\n", vmID, image)

	vm, err := manager.Create(ctx, vmID, image)
	if err != nil {
		return fmt.Errorf("failed to create microVM: %w", err)
	}

	fmt.Printf("MicroVM %s created successfully\n", vm.ID())
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

func testLifecycle(ctx context.Context, manager *microvm.Manager, vmID, image string) error {
	fmt.Printf("Testing microVM lifecycle with %s\n", image)

	// Create
	vm, err := manager.Create(ctx, vmID, image)
	if err != nil {
		return fmt.Errorf("failed to create microVM: %w", err)
	}
	fmt.Printf("✓ Created microVM %s\n", vm.ID())

	// Wait a bit
	fmt.Println("Waiting 5 seconds...")
	time.Sleep(5 * time.Second)

	// Check status
	status, err := vm.Status(ctx)
	if err != nil {
		return fmt.Errorf("failed to get status: %w", err)
	}
	fmt.Printf("✓ Status: %s\n", status)

	// Destroy
	err = manager.Destroy(ctx, vmID)
	if err != nil {
		return fmt.Errorf("failed to destroy microVM: %w", err)
	}
	fmt.Printf("✓ Destroyed microVM %s\n", vmID)

	fmt.Println("Test completed successfully!")
	return nil
}
