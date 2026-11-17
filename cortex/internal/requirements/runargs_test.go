package requirements

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuildRunArgs(t *testing.T) {
	tests := []struct {
		name string
		opts RunOpts
		want []string
	}{
		{
			name: "defaults share host network and ipc",
			opts: RunOpts{},
			want: []string{"run", "-d", "--name", "c1", "--network", "host", "--ipc", "host", "img:1"},
		},
		{
			name: "ipc opt-out",
			opts: RunOpts{IPC: IPCPrivate},
			want: []string{"run", "-d", "--name", "c1", "--network", "host", "--ipc", "private", "img:1"},
		},
		{
			name: "privileged devices volumes env kept, env sorted",
			opts: RunOpts{
				Privileged: true,
				Devices:    []string{"/dev/i2c-1"},
				Volumes:    []string{"/dev:/dev"},
				Env:        map[string]string{"B": "2", "A": "1"},
			},
			want: []string{
				"run", "-d", "--name", "c1", "--network", "host", "--ipc", "host",
				"--privileged", "--device", "/dev/i2c-1", "-v", "/dev:/dev",
				"-e", "A=1", "-e", "B=2", "img:1",
			},
		},
		{
			name: "shared memory forces host ipc and wires iceoryx",
			opts: RunOpts{IPC: IPCPrivate, SharedMemory: true, Env: map[string]string{"ROS_DOMAIN_ID": "42"}},
			want: []string{
				"run", "-d", "--name", "c1", "--network", "host", "--ipc", "host",
				"-v", "/tmp/iceoryx:/tmp",
				"-v", "/etc/zaraos/cyclonedds-shm.xml:/etc/cyclonedds-shm.xml:ro",
				"-e", "CYCLONEDDS_URI=file:///etc/cyclonedds-shm.xml",
				"-e", "ROS_DOMAIN_ID=42", "img:1",
			},
		},
		{
			name: "shared memory keeps user CYCLONEDDS_URI",
			opts: RunOpts{SharedMemory: true, Env: map[string]string{"CYCLONEDDS_URI": "file:///custom.xml"}},
			want: []string{
				"run", "-d", "--name", "c1", "--network", "host", "--ipc", "host",
				"-v", "/tmp/iceoryx:/tmp",
				"-v", "/etc/zaraos/cyclonedds-shm.xml:/etc/cyclonedds-shm.xml:ro",
				"-e", "CYCLONEDDS_URI=file:///custom.xml", "img:1",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildRunArgs("c1", "img:1", tt.opts)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("BuildRunArgs()\n got: %v\nwant: %v", got, tt.want)
			}
		})
	}
}

func TestBuildRunArgsDoesNotMutateEnv(t *testing.T) {
	env := map[string]string{"A": "1"}
	BuildRunArgs("c1", "img", RunOpts{SharedMemory: true, Env: env})
	if len(env) != 1 {
		t.Fatalf("caller env mutated: %v", env)
	}
}

func TestRunOptsFromPackage(t *testing.T) {
	p := Package{Privileged: true, Devices: []string{"d"}, Volumes: []string{"v"},
		Env: map[string]string{"k": "v"}, IPC: IPCShareable, SharedMemory: true}
	got := RunOptsFromPackage(p)
	want := RunOpts{Privileged: true, Devices: []string{"d"}, Volumes: []string{"v"},
		Env: map[string]string{"k": "v"}, IPC: IPCShareable, SharedMemory: true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v want %+v", got, want)
	}
}

func TestValidateIPC(t *testing.T) {
	base := func(p Package) *Manifest {
		p.Name, p.Image, p.Version = "a", "img", "latest"
		return &Manifest{Version: 1, Packages: []Package{p}}
	}
	tests := []struct {
		name    string
		pkg     Package
		wantErr string
	}{
		{"empty ok", Package{}, ""},
		{"host ok", Package{IPC: "host"}, ""},
		{"private ok", Package{IPC: "private"}, ""},
		{"shareable ok", Package{IPC: "shareable"}, ""},
		{"invalid", Package{IPC: "container:x"}, "invalid ipc"},
		{"shm with host ok", Package{IPC: "host", SharedMemory: true}, ""},
		{"shm with private rejected", Package{IPC: "private", SharedMemory: true}, "requires ipc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := base(tt.pkg).Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("want error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}
