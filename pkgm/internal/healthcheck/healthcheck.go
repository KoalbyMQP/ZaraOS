package healthcheck

import (
	"bufio"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Colors for terminal output
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
)

type CheckResult struct {
	Name   string
	Status string
	Error  error
}

func RunHealthCheck(dataDir string) error {
	fmt.Println("Running system health check...")
	fmt.Println()

	checks := []func(string) CheckResult{
		checkRootPrivileges,
		checkKVMSupport,
		checkKVMModules,
		checkMemoryAvailable,
		checkLinuxKernel,
		checkDevKVM,
		checkDevNetTun,
		checkFirecrackerBinary,
		checkContainerdShim,
		checkRuncBinary,
		checkDataDirWritable,
		checkDiskSpace,
		checkContainerdSocket,
		checkContainerdDaemon,
	}

	warningChecks := []func(string) CheckResult{
		checkIptables,
		checkBridgeUtils,
	}

	criticalFailed := 0
	warningFailed := 0

	// Run critical checks
	fmt.Println("Critical requirements:")
	for _, check := range checks {
		result := check(dataDir)
		printResult(result, false)
		if result.Status == "FAIL" {
			criticalFailed++
		}
	}

	fmt.Println()
	fmt.Println("Optional requirements:")
	// Run warning checks
	for _, check := range warningChecks {
		result := check(dataDir)
		printResult(result, true)
		if result.Status == "FAIL" {
			warningFailed++
		}
	}

	fmt.Println()
	if criticalFailed > 0 {
		fmt.Printf("%sSUMMARY: %d critical checks failed%s\n", colorRed, criticalFailed, colorReset)
		return fmt.Errorf("%d critical health checks failed", criticalFailed)
	} else {
		fmt.Printf("%sSUMMARY: All critical checks passed", colorGreen)
		if warningFailed > 0 {
			fmt.Printf(" (%d optional checks failed)", warningFailed)
		}
		fmt.Printf("%s\n", colorReset)
	}

	return nil
}

func printResult(result CheckResult, isWarning bool) {
	var statusColor string
	var prefix string

	if result.Status == "PASS" {
		statusColor = colorGreen
		prefix = "[PASS]"
	} else {
		if isWarning {
			statusColor = colorYellow
			prefix = "[WARN]"
		} else {
			statusColor = colorRed
			prefix = "[FAIL]"
		}
	}

	fmt.Printf("%s%-6s%s %s", statusColor, prefix, colorReset, result.Name)
	if result.Error != nil && result.Status == "FAIL" {
		fmt.Printf(" - %s", result.Error.Error())
	}
	fmt.Println()
}

func checkRootPrivileges(dataDir string) CheckResult {
	if os.Getuid() != 0 {
		return CheckResult{
			Name:   "Root privileges",
			Status: "FAIL",
			Error:  fmt.Errorf("must run as root"),
		}
	}
	return CheckResult{Name: "Root privileges", Status: "PASS"}
}

func checkKVMSupport(dataDir string) CheckResult {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return CheckResult{
			Name:   "CPU virtualization support",
			Status: "FAIL",
			Error:  fmt.Errorf("cannot read /proc/cpuinfo: %v", err),
		}
	}

	content := string(data)
	if strings.Contains(content, "vmx") || strings.Contains(content, "svm") {
		return CheckResult{Name: "CPU virtualization support", Status: "PASS"}
	}

	return CheckResult{
		Name:   "CPU virtualization support",
		Status: "FAIL",
		Error:  fmt.Errorf("no vmx/svm flags found in /proc/cpuinfo"),
	}
}

func checkKVMModules(dataDir string) CheckResult {
	data, err := os.ReadFile("/proc/modules")
	if err != nil {
		return CheckResult{
			Name:   "KVM kernel modules",
			Status: "FAIL",
			Error:  fmt.Errorf("cannot read /proc/modules: %v", err),
		}
	}

	content := string(data)
	if strings.Contains(content, "kvm") {
		return CheckResult{Name: "KVM kernel modules", Status: "PASS"}
	}

	return CheckResult{
		Name:   "KVM kernel modules",
		Status: "FAIL",
		Error:  fmt.Errorf("kvm modules not loaded"),
	}
}

func checkMemoryAvailable(dataDir string) CheckResult {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return CheckResult{
			Name:   "Available memory",
			Status: "FAIL",
			Error:  fmt.Errorf("cannot read /proc/meminfo: %v", err),
		}
	}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemAvailable:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				availKB, err := strconv.Atoi(fields[1])
				if err == nil {
					availMB := availKB / 1024
					if availMB >= 256 {
						return CheckResult{Name: "Available memory", Status: "PASS"}
					}
					return CheckResult{
						Name:   "Available memory",
						Status: "FAIL",
						Error:  fmt.Errorf("only %dMB available, need at least 256MB", availMB),
					}
				}
			}
		}
	}

	return CheckResult{
		Name:   "Available memory",
		Status: "FAIL",
		Error:  fmt.Errorf("cannot parse memory info"),
	}
}

func checkLinuxKernel(dataDir string) CheckResult {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return CheckResult{
			Name:   "Linux kernel",
			Status: "FAIL",
			Error:  fmt.Errorf("cannot read /proc/version: %v", err),
		}
	}

	if strings.Contains(string(data), "Linux") {
		return CheckResult{Name: "Linux kernel", Status: "PASS"}
	}

	return CheckResult{
		Name:   "Linux kernel",
		Status: "FAIL",
		Error:  fmt.Errorf("not running on Linux"),
	}
}

func checkDevKVM(dataDir string) CheckResult {
	info, err := os.Stat("/dev/kvm")
	if err != nil {
		return CheckResult{
			Name:   "/dev/kvm device",
			Status: "FAIL",
			Error:  fmt.Errorf("device not found: %v", err),
		}
	}

	// Check if it's a character device
	if info.Mode()&fs.ModeCharDevice == 0 {
		return CheckResult{
			Name:   "/dev/kvm device",
			Status: "FAIL",
			Error:  fmt.Errorf("not a character device"),
		}
	}

	// Try to open the device
	file, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
	if err != nil {
		return CheckResult{
			Name:   "/dev/kvm device",
			Status: "FAIL",
			Error:  fmt.Errorf("cannot access: %v", err),
		}
	}
	file.Close()

	return CheckResult{Name: "/dev/kvm device", Status: "PASS"}
}

func checkDevNetTun(dataDir string) CheckResult {
	_, err := os.Stat("/dev/net/tun")
	if err != nil {
		return CheckResult{
			Name:   "/dev/net/tun device",
			Status: "FAIL",
			Error:  fmt.Errorf("device not found: %v", err),
		}
	}
	return CheckResult{Name: "/dev/net/tun device", Status: "PASS"}
}

func checkFirecrackerBinary(dataDir string) CheckResult {
	// First check in assets dir
	assetPath := filepath.Join(dataDir, "assets", "firecracker")
	if info, err := os.Stat(assetPath); err == nil && info.Mode()&0111 != 0 {
		return CheckResult{Name: "Firecracker binary", Status: "PASS"}
	}

	// Check in PATH
	if _, err := exec.LookPath("firecracker"); err == nil {
		return CheckResult{Name: "Firecracker binary", Status: "PASS"}
	}

	return CheckResult{
		Name:   "Firecracker binary",
		Status: "FAIL",
		Error:  fmt.Errorf("not found in assets or PATH"),
	}
}

func checkContainerdShim(dataDir string) CheckResult {
	// First check in assets dir
	assetPath := filepath.Join(dataDir, "assets", "containerd-shim-aws-firecracker")
	if info, err := os.Stat(assetPath); err == nil && info.Mode()&0111 != 0 {
		return CheckResult{Name: "Containerd firecracker shim", Status: "PASS"}
	}

	// Check in PATH
	if _, err := exec.LookPath("containerd-shim-aws-firecracker"); err == nil {
		return CheckResult{Name: "Containerd firecracker shim", Status: "PASS"}
	}

	return CheckResult{
		Name:   "Containerd firecracker shim",
		Status: "FAIL",
		Error:  fmt.Errorf("not found in assets or PATH"),
	}
}

func checkRuncBinary(dataDir string) CheckResult {
	if _, err := exec.LookPath("runc"); err == nil {
		return CheckResult{Name: "Runc runtime", Status: "PASS"}
	}
	return CheckResult{
		Name:   "Runc runtime",
		Status: "FAIL",
		Error:  fmt.Errorf("runc not found in PATH"),
	}
}

func checkDataDirWritable(dataDir string) CheckResult {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return CheckResult{
			Name:   "Data directory writable",
			Status: "FAIL",
			Error:  fmt.Errorf("cannot create directory: %v", err),
		}
	}

	// Test write
	testFile := filepath.Join(dataDir, ".healthcheck-test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		return CheckResult{
			Name:   "Data directory writable",
			Status: "FAIL",
			Error:  fmt.Errorf("cannot write to directory: %v", err),
		}
	}

	// Clean up
	os.Remove(testFile)
	return CheckResult{Name: "Data directory writable", Status: "PASS"}
}

func checkDiskSpace(dataDir string) CheckResult {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(dataDir, &stat); err != nil {
		return CheckResult{
			Name:   "Sufficient disk space",
			Status: "FAIL",
			Error:  fmt.Errorf("cannot check disk space: %v", err),
		}
	}

	// Calculate available space in MB
	availBytes := stat.Bavail * uint64(stat.Bsize)
	availMB := availBytes / (1024 * 1024)

	if availMB >= 1024 { // At least 1GB
		return CheckResult{Name: "Sufficient disk space", Status: "PASS"}
	}

	return CheckResult{
		Name:   "Sufficient disk space",
		Status: "FAIL",
		Error:  fmt.Errorf("only %dMB available, recommend at least 1GB", availMB),
	}
}

func checkContainerdSocket(dataDir string) CheckResult {
	socketPath := filepath.Join(dataDir, "containerd.sock")
	
	// Check if socket file exists
	if _, err := os.Stat(socketPath); err != nil {
		// Also check system socket
		if _, err := os.Stat("/run/containerd/containerd.sock"); err != nil {
			return CheckResult{
				Name:   "Containerd socket",
				Status: "FAIL",
				Error:  fmt.Errorf("socket not found"),
			}
		}
		socketPath = "/run/containerd/containerd.sock"
	}

	return CheckResult{Name: "Containerd socket", Status: "PASS"}
}

func checkContainerdDaemon(dataDir string) CheckResult {
	socketPath := filepath.Join(dataDir, "containerd.sock")
	
	// Check if local socket exists, otherwise use system socket
	if _, err := os.Stat(socketPath); err != nil {
		socketPath = "/run/containerd/containerd.sock"
	}

	// Try to connect to the socket
	conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	if err != nil {
		return CheckResult{
			Name:   "Containerd daemon responsive",
			Status: "FAIL",
			Error:  fmt.Errorf("cannot connect to containerd: %v", err),
		}
	}
	conn.Close()

	return CheckResult{Name: "Containerd daemon responsive", Status: "PASS"}
}

func checkIptables(dataDir string) CheckResult {
	if _, err := exec.LookPath("iptables"); err == nil {
		return CheckResult{Name: "Iptables available", Status: "PASS"}
	}
	return CheckResult{
		Name:   "Iptables available",
		Status: "FAIL",
		Error:  fmt.Errorf("iptables not found in PATH"),
	}
}

func checkBridgeUtils(dataDir string) CheckResult {
	// Check for modern 'ip' command
	if _, err := exec.LookPath("ip"); err == nil {
		return CheckResult{Name: "Bridge utilities", Status: "PASS"}
	}

	// Check for legacy 'brctl' command
	if _, err := exec.LookPath("brctl"); err == nil {
		return CheckResult{Name: "Bridge utilities", Status: "PASS"}
	}

	return CheckResult{
		Name:   "Bridge utilities",
		Status: "FAIL",
		Error:  fmt.Errorf("neither 'ip' nor 'brctl' found in PATH"),
	}
}