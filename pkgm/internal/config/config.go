package config

type Config struct {
	ContainerdSocket string            `json:"containerd_socket"`
	Namespace        string            `json:"namespace"`
	Runtime          string            `json:"runtime"`
	DefaultResources ResourceLimits    `json:"default_resources"`
	Images           map[string]string `json:"images"`
}

type ResourceLimits struct {
	MemoryMB int64 `json:"memory_mb"`
	CPUs     int64 `json:"cpus"`
}

func Default() *Config {
	return &Config{
		ContainerdSocket: "/run/containerd/containerd.sock",
		Namespace:        "firecracker-manager",
		Runtime:          "aws.firecracker",
		DefaultResources: ResourceLimits{
			MemoryMB: 128,
			CPUs:     1,
		},
		Images: map[string]string{
			"alpine": "alpine:latest",
			"ubuntu": "ubuntu:22.04",
		},
	}
}
