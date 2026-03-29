package firecracker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

// Client communicates with the Firecracker API over a Unix socket.
type Client interface {
	PutBootSource(cfg BootSource) error
	PutDrive(id string, cfg Drive) error
	PutNetworkInterface(id string, cfg NetworkInterface) error
	PutMachineConfig(cfg MachineConfig) error
	StartInstance() error
	PauseVM() error
	ResumeVM() error
	CreateSnapshot(cfg SnapshotCreate) error
	LoadSnapshot(cfg SnapshotLoad) error
}

// BootSource configures the kernel and boot arguments.
type BootSource struct {
	KernelImagePath string `json:"kernel_image_path"`
	BootArgs        string `json:"boot_args"`
}

// Drive configures a block device.
type Drive struct {
	DriveID      string `json:"drive_id"`
	PathOnHost   string `json:"path_on_host"`
	IsRootDevice bool   `json:"is_root_device"`
	IsReadOnly   bool   `json:"is_read_only"`
}

// NetworkInterface configures a network device.
type NetworkInterface struct {
	IfaceID     string `json:"iface_id"`
	GuestMAC    string `json:"guest_mac"`
	HostDevName string `json:"host_dev_name"`
}

// MachineConfig sets CPU and memory.
type MachineConfig struct {
	VCPUCount  int `json:"vcpu_count"`
	MemSizeMiB int `json:"mem_size_mib"`
}

// SnapshotCreate configures a snapshot operation.
type SnapshotCreate struct {
	SnapshotType string `json:"snapshot_type"`
	SnapshotPath string `json:"snapshot_path"`
	MemFilePath  string `json:"mem_file_path"`
}

// SnapshotLoad configures loading a snapshot.
type SnapshotLoad struct {
	SnapshotPath string     `json:"snapshot_path"`
	MemBackend   MemBackend `json:"mem_backend"`
}

// MemBackend specifies where to load memory from.
type MemBackend struct {
	BackendPath string `json:"backend_path"`
	BackendType string `json:"backend_type"`
}

// HTTPClient is the real implementation that talks to Firecracker over a Unix socket.
type HTTPClient struct {
	socketPath string
	client     *http.Client
}

// NewClient creates a new Firecracker API client.
func NewClient(socketPath string) *HTTPClient {
	return &HTTPClient{
		socketPath: socketPath,
		client: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", socketPath)
				},
			},
			Timeout: 30 * time.Second,
		},
	}
}

// WaitForSocket polls until the Firecracker socket is ready.
func WaitForSocket(socketPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(socketPath); err == nil {
			// Socket file exists, try connecting
			conn, err := net.DialTimeout("unix", socketPath, 500*time.Millisecond)
			if err == nil {
				conn.Close()
				return nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for firecracker socket %s after %v", socketPath, timeout)
}

func (c *HTTPClient) PutBootSource(cfg BootSource) error {
	return c.put("/boot-source", cfg)
}

func (c *HTTPClient) PutDrive(id string, cfg Drive) error {
	return c.put(fmt.Sprintf("/drives/%s", id), cfg)
}

func (c *HTTPClient) PutNetworkInterface(id string, cfg NetworkInterface) error {
	return c.put(fmt.Sprintf("/network-interfaces/%s", id), cfg)
}

func (c *HTTPClient) PutMachineConfig(cfg MachineConfig) error {
	return c.put("/machine-config", cfg)
}

func (c *HTTPClient) StartInstance() error {
	return c.put("/actions", map[string]string{"action_type": "InstanceStart"})
}

func (c *HTTPClient) PauseVM() error {
	return c.patch("/vm", map[string]string{"state": "Paused"})
}

func (c *HTTPClient) ResumeVM() error {
	return c.patch("/vm", map[string]string{"state": "Resumed"})
}

func (c *HTTPClient) CreateSnapshot(cfg SnapshotCreate) error {
	return c.put("/snapshot/create", cfg)
}

func (c *HTTPClient) LoadSnapshot(cfg SnapshotLoad) error {
	return c.put("/snapshot/load", cfg)
}

func (c *HTTPClient) put(path string, body interface{}) error {
	return c.do("PUT", path, body)
}

func (c *HTTPClient) patch(path string, body interface{}) error {
	return c.do("PATCH", path, body)
}

func (c *HTTPClient) do(method, path string, body interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	url := "http://localhost" + path
	req, err := http.NewRequest(method, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("firecracker api %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("firecracker api %s %s: status %d: %s",
			method, path, resp.StatusCode, string(respBody))
	}

	return nil
}
