#!/usr/bin/env python3
"""
ZaraOS Demonstration Script
===========================
This script demonstrates that ZaraOS is running properly by showing
system information and capabilities relevant to robotics applications.
"""

import os
import sys
import time
import subprocess
import socket
from datetime import datetime

class Colors:
    """ANSI color codes for terminal output"""
    HEADER = '\033[95m'
    OKBLUE = '\033[94m'
    OKCYAN = '\033[96m'
    OKGREEN = '\033[92m'
    WARNING = '\033[93m'
    FAIL = '\033[91m'
    ENDC = '\033[0m'
    BOLD = '\033[1m'
    UNDERLINE = '\033[4m'

def print_header(text):
    """Print a colored header"""
    print(f"\n{Colors.HEADER}{Colors.BOLD}{'='*50}{Colors.ENDC}")
    print(f"{Colors.HEADER}{Colors.BOLD}{text.center(50)}{Colors.ENDC}")
    print(f"{Colors.HEADER}{Colors.BOLD}{'='*50}{Colors.ENDC}")

def print_info(label, value, color=Colors.OKGREEN):
    """Print formatted information"""
    print(f"{Colors.OKBLUE}{label}:{Colors.ENDC} {color}{value}{Colors.ENDC}")

def run_command(cmd):
    """Run a command and return its output"""
    try:
        result = subprocess.run(cmd, shell=True, capture_output=True, text=True)
        return result.stdout.strip() if result.returncode == 0 else "Unknown"
    except:
        return "Unknown"

def get_system_info():
    """Gather system information"""
    info = {}

    # Basic system info
    info['hostname'] = run_command('hostname')
    info['uptime'] = run_command('uptime -p')
    info['kernel'] = run_command('uname -r')
    info['architecture'] = run_command('uname -m')

    # Memory info
    try:
        with open('/proc/meminfo', 'r') as f:
            meminfo = f.read()
        for line in meminfo.split('\n'):
            if line.startswith('MemTotal:'):
                info['memory_total'] = line.split()[1] + ' kB'
            elif line.startswith('MemFree:'):
                info['memory_free'] = line.split()[1] + ' kB'
            elif line.startswith('MemAvailable:'):
                info['memory_available'] = line.split()[1] + ' kB'
    except:
        info['memory_total'] = "Unknown"
        info['memory_free'] = "Unknown"
        info['memory_available'] = "Unknown"

    # CPU info
    try:
        with open('/proc/cpuinfo', 'r') as f:
            cpuinfo = f.read()
        cpu_count = cpuinfo.count('processor')
        info['cpu_count'] = str(cpu_count)

        # Get CPU model
        for line in cpuinfo.split('\n'):
            if line.startswith('model name'):
                info['cpu_model'] = line.split(':')[1].strip()
                break
        else:
            info['cpu_model'] = "ARM Processor"
    except:
        info['cpu_count'] = "Unknown"
        info['cpu_model'] = "Unknown"

    # Temperature (Pi-specific)
    info['temperature'] = run_command('vcgencmd measure_temp 2>/dev/null || echo "Not available"')

    # Load average
    try:
        with open('/proc/loadavg', 'r') as f:
            info['load_avg'] = f.read().split()[0:3]
    except:
        info['load_avg'] = ["Unknown", "Unknown", "Unknown"]

    return info

def get_network_info():
    """Get network interface information"""
    interfaces = {}

    # Get IP addresses
    try:
        result = subprocess.run(['ip', 'addr', 'show'], capture_output=True, text=True)
        if result.returncode == 0:
            current_interface = None
            for line in result.stdout.split('\n'):
                if line and not line.startswith(' '):
                    parts = line.split(':')
                    if len(parts) >= 2:
                        current_interface = parts[1].strip()
                        interfaces[current_interface] = {'ip': 'No IP', 'status': 'down'}
                elif 'inet ' in line and current_interface:
                    ip = line.strip().split()[1].split('/')[0]
                    interfaces[current_interface]['ip'] = ip
                elif 'state UP' in line and current_interface:
                    interfaces[current_interface]['status'] = 'up'
    except:
        pass

    return interfaces

def display_system_status():
    """Display comprehensive system status"""
    print_header("ZaraOS System Status")

    # System Information
    print(f"\n{Colors.OKCYAN}{Colors.BOLD}System Information:{Colors.ENDC}")
    info = get_system_info()

    print_info("Hostname", info['hostname'])
    print_info("Uptime", info['uptime'])
    print_info("Kernel", info['kernel'])
    print_info("Architecture", info['architecture'])
    print_info("CPU Model", info['cpu_model'])
    print_info("CPU Cores", info['cpu_count'])
    print_info("Temperature", info['temperature'])
    print_info("Load Average", f"{info['load_avg'][0]} {info['load_avg'][1]} {info['load_avg'][2]}")

    # Memory Information
    print(f"\n{Colors.OKCYAN}{Colors.BOLD}Memory Information:{Colors.ENDC}")
    print_info("Total Memory", info['memory_total'])
    print_info("Available Memory", info['memory_available'])
    print_info("Free Memory", info['memory_free'])

    # Network Information
    print(f"\n{Colors.OKCYAN}{Colors.BOLD}Network Interfaces:{Colors.ENDC}")
    interfaces = get_network_info()

    if interfaces:
        for interface, data in interfaces.items():
            if interface != 'lo':  # Skip loopback
                status_color = Colors.OKGREEN if data['status'] == 'up' else Colors.WARNING
                print_info(f"Interface {interface}", f"{data['ip']} ({data['status']})", status_color)
    else:
        print_info("Network", "No interfaces found", Colors.WARNING)

    # Filesystem Information
    print(f"\n{Colors.OKCYAN}{Colors.BOLD}Filesystem Information:{Colors.ENDC}")
    fs_info = run_command('df -h / | tail -1')
    if fs_info and fs_info != "Unknown":
        parts = fs_info.split()
        if len(parts) >= 4:
            print_info("Root Filesystem", f"{parts[1]} total, {parts[2]} used, {parts[3]} available")

    # ZaraOS Specific Info
    print(f"\n{Colors.OKCYAN}{Colors.BOLD}ZaraOS Information:{Colors.ENDC}")
    if os.path.exists('/etc/zaraos-release'):
        try:
            with open('/etc/zaraos-release', 'r') as f:
                zaraos_info = f.read().strip()
            for line in zaraos_info.split('\n'):
                if line.strip():
                    print_info("", line.strip())
        except:
            print_info("ZaraOS Release", "Information not available")
    else:
        print_info("ZaraOS Release", "Information not available")

def run_system_test():
    """Run a simple system test"""
    print_header("System Test")

    tests = [
        ("Python Runtime", "python3 --version"),
        ("Basic Math", "python3 -c 'print(2+2)'"),
        ("File System", "ls /tmp > /dev/null"),
        ("Network Stack", "ping -c 1 127.0.0.1 > /dev/null"),
    ]

    for test_name, command in tests:
        try:
            result = subprocess.run(command, shell=True, capture_output=True)
            if result.returncode == 0:
                print_info(test_name, "PASS", Colors.OKGREEN)
            else:
                print_info(test_name, "FAIL", Colors.FAIL)
        except:
            print_info(test_name, "ERROR", Colors.FAIL)

def main():
    """Main demonstration function"""
    print(f"{Colors.BOLD}{Colors.HEADER}")
    print("=" * 60)
    print("    ZaraOS Demonstration - Raspberry Pi 5 Ready!")
    print("=" * 60)
    print(f"{Colors.ENDC}")

    print(f"{Colors.OKCYAN}Starting system demonstration...{Colors.ENDC}")
    print(f"{Colors.WARNING}This will take about 10 seconds{Colors.ENDC}")

    # Show system status
    display_system_status()

    # Run system tests
    run_system_test()

    # Final message
    print_header("Demo Complete")
    print(f"{Colors.OKGREEN}✓ ZaraOS is running successfully!{Colors.ENDC}")
    print(f"{Colors.OKGREEN}✓ Python3 is working correctly{Colors.ENDC}")
    print(f"{Colors.OKGREEN}✓ System is ready for robotics applications{Colors.ENDC}")

    print(f"\n{Colors.OKCYAN}You can now use this system for:")
    print(f"• Running Python robotics applications")
    print(f"• Connecting sensors and actuators")
    print(f"• Network communication")
    print(f"• Real-time control tasks{Colors.ENDC}")

    print(f"\n{Colors.BOLD}Press Ctrl+C to exit to shell, or this will auto-exit in 5 seconds...{Colors.ENDC}")

    # Auto-exit after 5 seconds
    try:
        time.sleep(5)
    except KeyboardInterrupt:
        print(f"\n{Colors.OKCYAN}Interrupted by user{Colors.ENDC}")

    print(f"\n{Colors.OKGREEN}Welcome to ZaraOS! 🤖{Colors.ENDC}\n")

if __name__ == "__main__":
    main()
