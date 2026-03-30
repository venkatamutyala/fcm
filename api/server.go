package api

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"fcm.dev/fcm-cli/internal/cloudinit"
	"fcm.dev/fcm-cli/internal/config"
	"fcm.dev/fcm-cli/internal/firecracker"
	"fcm.dev/fcm-cli/internal/images"
	"fcm.dev/fcm-cli/internal/network"
	"fcm.dev/fcm-cli/internal/systemd"
	"fcm.dev/fcm-cli/internal/templates"
	"fcm.dev/fcm-cli/internal/vm"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Server is the FCM HTTP API server.
type Server struct {
	Token  string
	router chi.Router
}

// NewServer creates a new API server with the given bearer token for auth.
func NewServer(token string) *Server {
	s := &Server{Token: token}
	s.router = s.buildRouter()
	return s
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) buildRouter() chi.Router {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.SetHeader("Content-Type", "application/json"))
	r.Use(limitBodySize(1 << 20)) // 1 MB

	r.Route("/v1", func(r chi.Router) {
		r.Get("/health", s.handleHealth)

		// Authenticated routes
		r.Group(func(r chi.Router) {
			r.Use(s.authMiddleware)

			r.Get("/vms", s.handleListVMs)
			r.Post("/vms", s.handleCreateVM)
			r.Get("/vms/{name}", s.handleInspectVM)
			r.Delete("/vms/{name}", s.handleDeleteVM)
			r.Post("/vms/{name}/freeze", s.handleFreezeVM)
			r.Post("/vms/{name}/unfreeze", s.handleUnfreezeVM)
			r.Post("/vms/{name}/exec", s.handleExecVM)

			r.Get("/images", s.handleListImages)
			r.Post("/images/{name}/pull", s.handlePullImage)

			r.Get("/templates", s.handleListTemplates)
		})
	})

	return r
}

// limitBodySize returns middleware that limits request body size.
func limitBodySize(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// authMiddleware checks the Authorization: Bearer <token> header.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "missing or invalid Authorization header")
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		if token != s.Token {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- Response helpers ---

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// --- Handlers ---

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleListVMs(w http.ResponseWriter, r *http.Request) {
	vms, err := vm.LoadAllVMs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	type vmEntry struct {
		Name     string `json:"name"`
		Status   string `json:"status"`
		IP       string `json:"ip"`
		CPUs     int    `json:"cpus"`
		MemoryMB int    `json:"memory_mb"`
		DiskGB   int    `json:"disk_gb"`
		Image    string `json:"image"`
	}

	entries := make([]vmEntry, 0, len(vms))
	for _, v := range vms {
		entries = append(entries, vmEntry{
			Name:     v.Name,
			Status:   systemd.VMStatus(v.Name),
			IP:       v.IP,
			CPUs:     v.CPUs,
			MemoryMB: v.MemoryMB,
			DiskGB:   v.DiskGB,
			Image:    v.Image,
		})
	}

	writeJSON(w, http.StatusOK, entries)
}

type createRequest struct {
	Name      string `json:"name"`
	Image     string `json:"image"`
	CPUs      int    `json:"cpus"`
	Memory    int    `json:"memory"`
	Disk      int    `json:"disk"`
	SSHKey    string `json:"ssh_key"`
	Template  string `json:"template"`
	CloudInit string `json:"cloud_init"`
}

func (s *Server) handleCreateVM(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := vm.ValidateName(req.Name); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if vm.Exists(req.Name) {
		writeError(w, http.StatusConflict, fmt.Sprintf("vm %q already exists", req.Name))
		return
	}

	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("load config: %v", err))
		return
	}

	if !network.BridgeExists(cfg.BridgeName) {
		writeError(w, http.StatusServiceUnavailable, fmt.Sprintf("network bridge %s not found (run fcm init first)", cfg.BridgeName))
		return
	}

	// Resolve template
	var tmpl *templates.Template
	if req.Template != "" {
		tmpl = templates.Get(req.Template)
		if tmpl == nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown template %q", req.Template))
			return
		}
		if req.Image == "" {
			req.Image = tmpl.Image
		}
	}

	if req.Image == "" {
		req.Image = "ubuntu-24.04"
	}

	// Apply defaults
	cpus := req.CPUs
	if cpus == 0 {
		if tmpl != nil && tmpl.CPUs > 0 {
			cpus = tmpl.CPUs
		} else {
			cpus = cfg.DefaultCPUs
		}
	}
	memory := req.Memory
	if memory == 0 {
		if tmpl != nil && tmpl.Memory > 0 {
			memory = tmpl.Memory
		} else {
			memory = cfg.DefaultMemoryMB
		}
	}
	disk := req.Disk
	if disk == 0 {
		if tmpl != nil && tmpl.Disk > 0 {
			disk = tmpl.Disk
		} else {
			disk = cfg.DefaultDiskGB
		}
	}

	// Validate
	if cpus < 1 || cpus > 32 {
		writeError(w, http.StatusBadRequest, "cpus must be between 1 and 32")
		return
	}
	if memory < 128 {
		writeError(w, http.StatusBadRequest, "memory must be at least 128 MB")
		return
	}
	if disk < 1 {
		writeError(w, http.StatusBadRequest, "disk must be at least 1 GB")
		return
	}

	// Auto-pull image if not cached
	if !images.Exists(req.Image) {
		if err := images.Pull(req.Image); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("pull image: %v", err))
			return
		}
	}

	// Create VM under lock
	var v *vm.VM
	err = vm.WithLock(func() error {
		ip, err := network.AllocateIP(cfg)
		if err != nil {
			return err
		}

		mac := network.MACFromIP(ip)
		tapDevice := network.TAPName(req.Name)
		vmDir := vm.VMDir(req.Name)

		v = &vm.VM{
			Name:       req.Name,
			Image:      req.Image,
			Kernel:     cfg.DefaultKernel,
			CPUs:       cpus,
			MemoryMB:   memory,
			DiskGB:     disk,
			IP:         ip,
			Gateway:    cfg.BridgeIP,
			MAC:        mac,
			TAPDevice:  tapDevice,
			SocketPath: filepath.Join(vmDir, "fc.socket"),
			RootfsPath: filepath.Join(vmDir, "rootfs.ext4"),
			CIDataPath: filepath.Join(vmDir, "cidata.iso"),
			SerialLog:  filepath.Join(vmDir, "console.log"),
			CreatedAt:  time.Now(),
			BootArgs:   network.BootArgs(ip, cfg.BridgeIP, cfg.BridgeMask),
		}

		if err := os.MkdirAll(vmDir, 0700); err != nil {
			return fmt.Errorf("create vm dir: %w", err)
		}

		if err := vm.SaveVM(v); err != nil {
			os.RemoveAll(vmDir)
			return err
		}

		return nil
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	rollback := func() {
		_ = os.RemoveAll(vm.VMDir(req.Name))
		_ = systemd.RemoveVMUnit(req.Name)
	}

	// Copy and resize image
	if err := images.CopyForVM(req.Image, v.RootfsPath, disk); err != nil {
		rollback()
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("prepare rootfs: %v", err))
		return
	}

	// Generate cloud-init
	sshPubKey := req.SSHKey
	var cloudInitFile string

	if cloudInitFile == "" && tmpl != nil && tmpl.CloudInit != "" {
		baseUserData := cloudinit.DefaultUserData(req.Name, sshPubKey)
		merged := templates.MergeCloudInit(baseUserData, tmpl.CloudInit)

		tmpFile, err := os.CreateTemp("", "fcm-api-ci-*.yaml")
		if err != nil {
			rollback()
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("create temp cloud-init: %v", err))
			return
		}
		defer os.Remove(tmpFile.Name())

		if _, err := tmpFile.WriteString(merged); err != nil {
			tmpFile.Close()
			rollback()
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("write temp cloud-init: %v", err))
			return
		}
		tmpFile.Close()
		cloudInitFile = tmpFile.Name()
	}

	// Handle raw cloud_init content from request body
	if req.CloudInit != "" && cloudInitFile == "" {
		tmpFile, err := os.CreateTemp("", "fcm-api-ci-*.yaml")
		if err != nil {
			rollback()
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("create temp cloud-init: %v", err))
			return
		}
		defer os.Remove(tmpFile.Name())

		if _, err := tmpFile.WriteString(req.CloudInit); err != nil {
			tmpFile.Close()
			rollback()
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("write cloud-init: %v", err))
			return
		}
		tmpFile.Close()
		cloudInitFile = tmpFile.Name()
	}

	netCfg := &cloudinit.NetworkConfig{
		IP:      v.IP,
		Gateway: v.Gateway,
		Mask:    cfg.BridgeMask,
		DNS:     cfg.DNS,
	}
	if err := cloudinit.GenerateCloudInitDisk(v.CIDataPath, req.Name, sshPubKey, cloudInitFile, netCfg); err != nil {
		rollback()
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("generate cloud-init: %v", err))
		return
	}

	_ = os.WriteFile(v.SerialLog, nil, 0600)

	if err := systemd.WriteVMUnit(v); err != nil {
		rollback()
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("write systemd unit: %v", err))
		return
	}

	unit := systemd.VMUnitName(req.Name)
	if err := systemd.Enable(unit); err != nil {
		rollback()
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("enable unit: %v", err))
		return
	}

	if err := systemd.Start(unit); err != nil {
		rollback()
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("start vm: %v", err))
		return
	}

	// Wait for SSH readiness (up to 120s)
	waitForSSH(v.IP, 120*time.Second)

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"name":      v.Name,
		"status":    "running",
		"ip":        v.IP,
		"cpus":      v.CPUs,
		"memory_mb": v.MemoryMB,
		"disk_gb":   v.DiskGB,
		"image":     v.Image,
	})
}

func (s *Server) handleInspectVM(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	v, err := vm.LoadVM(name)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("vm %q not found", name))
		return
	}

	type inspectOutput struct {
		*vm.VM
		Status string `json:"status"`
	}

	writeJSON(w, http.StatusOK, inspectOutput{
		VM:     v,
		Status: systemd.VMStatus(name),
	})
}

func (s *Server) handleDeleteVM(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if !vm.Exists(name) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("vm %q not found", name))
		return
	}

	force := r.URL.Query().Get("force") == "true"

	unit := systemd.VMUnitName(name)
	if systemd.IsActive(unit) {
		if !force {
			writeError(w, http.StatusConflict, fmt.Sprintf("vm %q is running (use ?force=true to force delete)", name))
			return
		}
		if err := systemd.Stop(unit); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("stop vm: %v", err))
			return
		}
	}

	// Clean up TAP device
	v, err := vm.LoadVM(name)
	if err == nil {
		_ = network.DeleteTAP(v.TAPDevice)
	}

	if err := systemd.RemoveVMUnit(name); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("remove systemd unit: %v", err))
		return
	}

	if err := vm.DeleteVMState(name); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("delete vm data: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "name": name})
}

func (s *Server) handleFreezeVM(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	v, err := vm.LoadVM(name)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("vm %q not found", name))
		return
	}

	unit := systemd.VMUnitName(name)
	if !systemd.IsActive(unit) {
		writeError(w, http.StatusConflict, fmt.Sprintf("vm %q is not running", name))
		return
	}

	fc := firecracker.NewClient(v.SocketPath)

	if err := fc.PauseVM(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("pause: %v", err))
		return
	}

	vmDir := vm.VMDir(name)
	snapPath := filepath.Join(vmDir, "snapshot.snap")
	memPath := filepath.Join(vmDir, "snapshot.mem")

	if err := fc.CreateSnapshot(firecracker.SnapshotCreate{
		SnapshotType: "Full",
		SnapshotPath: snapPath,
		MemFilePath:  memPath,
	}); err != nil {
		_ = fc.ResumeVM()
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("snapshot: %v", err))
		return
	}

	if err := systemd.Stop(unit); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("stop: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "frozen", "name": name})
}

func (s *Server) handleUnfreezeVM(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if !vm.Exists(name) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("vm %q not found", name))
		return
	}

	vmDir := vm.VMDir(name)
	snapPath := filepath.Join(vmDir, "snapshot.snap")
	if _, err := os.Stat(snapPath); os.IsNotExist(err) {
		writeError(w, http.StatusConflict, fmt.Sprintf("vm %q has no snapshot (freeze it first)", name))
		return
	}

	unit := systemd.VMUnitName(name)
	if systemd.IsActive(unit) {
		writeError(w, http.StatusConflict, fmt.Sprintf("vm %q is already running", name))
		return
	}

	if err := systemd.Start(unit); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("start: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "running", "name": name})
}

type execRequest struct {
	Command string `json:"command"`
}

type execResponse struct {
	Output   string `json:"output"`
	ExitCode int    `json:"exit_code"`
}

func (s *Server) handleExecVM(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	v, err := vm.LoadVM(name)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("vm %q not found", name))
		return
	}

	if v.IP == "" {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("vm %q has no IP address", name))
		return
	}

	var req execRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Command == "" {
		writeError(w, http.StatusBadRequest, "command is required")
		return
	}

	sshArgs := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
		"-o", "ConnectTimeout=10",
	}

	// Try to find an SSH key
	if key := findSSHKey(); key != "" {
		sshArgs = append(sshArgs, "-o", "IdentitiesOnly=yes", "-i", key)
	}

	sshArgs = append(sshArgs, "root@"+v.IP, req.Command)

	cmd := exec.Command("ssh", sshArgs...)
	output, err := cmd.CombinedOutput()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("ssh exec: %v", err))
			return
		}
	}

	writeJSON(w, http.StatusOK, execResponse{
		Output:   string(output),
		ExitCode: exitCode,
	})
}

func (s *Server) handleListImages(w http.ResponseWriter, r *http.Request) {
	imgs, err := images.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	type imageEntry struct {
		Name string `json:"name"`
		Size int64  `json:"size_bytes"`
	}

	entries := make([]imageEntry, 0, len(imgs))
	for _, img := range imgs {
		entries = append(entries, imageEntry{
			Name: img.Name,
			Size: img.Size,
		})
	}

	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) handlePullImage(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if images.Exists(name) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "already_exists", "name": name})
		return
	}

	if err := images.Pull(name); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("pull image: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "pulled", "name": name})
}

func (s *Server) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	tmplList := templates.List()

	type entry struct {
		Name        string `json:"name"`
		Image       string `json:"image"`
		Description string `json:"description"`
		CPUs        int    `json:"cpus,omitempty"`
		MemoryMB    int    `json:"memory_mb,omitempty"`
		DiskGB      int    `json:"disk_gb,omitempty"`
	}

	entries := make([]entry, 0, len(tmplList))
	for _, t := range tmplList {
		entries = append(entries, entry{
			Name:        t.Name,
			Image:       t.Image,
			Description: t.Description,
			CPUs:        t.CPUs,
			MemoryMB:    t.Memory,
			DiskGB:      t.Disk,
		})
	}

	writeJSON(w, http.StatusOK, entries)
}

// --- Helpers ---

// findSSHKey looks for common SSH private key files.
func findSSHKey() string {
	home := os.Getenv("HOME")
	candidates := []string{
		home + "/.ssh/id_ed25519",
		home + "/.ssh/id_rsa",
		home + "/.ssh/id_ecdsa",
		"/root/.ssh/id_ed25519",
		"/root/.ssh/id_rsa",
		"/root/.ssh/id_ecdsa",
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// waitForSSH polls TCP port 22 until reachable or timeout expires.
func waitForSSH(ip string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := (&net.Dialer{Timeout: 2 * time.Second}).Dial("tcp", net.JoinHostPort(ip, "22"))
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(2 * time.Second)
	}
}
