package images

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	imagesDir = "/var/lib/fcm/images"
	cacheDir  = "/var/lib/fcm/cache"
)

// Image represents a locally available rootfs image.
type Image struct {
	Name string
	Path string
	Size int64
}

// List returns all locally available images.
func List() ([]Image, error) {
	entries, err := os.ReadDir(imagesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list images: %w", err)
	}

	var images []Image
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".ext4") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".ext4")
		images = append(images, Image{
			Name: name,
			Path: filepath.Join(imagesDir, e.Name()),
			Size: info.Size(),
		})
	}
	return images, nil
}

// ImagePath returns the path to a named image.
func ImagePath(name string) string {
	return filepath.Join(imagesDir, name+".ext4")
}

// Exists checks if a named image exists locally.
func Exists(name string) bool {
	_, err := os.Stat(ImagePath(name))
	return err == nil
}

// Pull downloads a Ubuntu cloud image (qcow2), converts to raw, and extracts
// the root partition as a standalone ext4 file.
// Requires: qemu-img, sfdisk, e2fsck (one-time pull dependencies).
func Pull(name string) error {
	if err := checkPullDeps(); err != nil {
		return err
	}

	if err := os.MkdirAll(imagesDir, 0700); err != nil {
		return fmt.Errorf("create images dir: %w", err)
	}
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}

	url, err := resolveImageURL(name)
	if err != nil {
		return err
	}

	destPath := ImagePath(name)
	qcowPath := filepath.Join(cacheDir, name+".img")
	rawPath := filepath.Join(cacheDir, name+".raw")

	// Step 1: Download qcow2
	fmt.Printf("[1/4] Downloading %s cloud image...\n", name)
	if err := downloadFile(url, qcowPath); err != nil {
		os.Remove(qcowPath)
		return fmt.Errorf("download: %w", err)
	}

	// Step 2: Convert qcow2 → raw
	fmt.Println("[2/4] Converting qcow2 to raw...")
	if out, err := exec.Command("qemu-img", "convert", "-f", "qcow2", "-O", "raw", qcowPath, rawPath).CombinedOutput(); err != nil {
		cleanup(qcowPath, rawPath)
		return fmt.Errorf("qemu-img convert: %s: %w", string(out), err)
	}
	os.Remove(qcowPath)

	// Step 3: Extract root partition
	fmt.Println("[3/4] Extracting root partition...")
	if err := extractRootPartition(rawPath, destPath); err != nil {
		cleanup(rawPath, destPath)
		return fmt.Errorf("extract rootfs: %w", err)
	}
	os.Remove(rawPath)

	// Step 4: Fix rootfs for Firecracker (remove references to boot/EFI partitions)
	fmt.Println("[4/5] Patching rootfs for Firecracker...")
	if err := patchRootfsForFirecracker(destPath); err != nil {
		os.Remove(destPath)
		return fmt.Errorf("patch rootfs: %w", err)
	}

	// Step 5: Verify filesystem (ext4 or xfs)
	fmt.Println("[5/5] Verifying filesystem...")
	fsType := detectFS(destPath)
	if fsType == "ext4" || fsType == "ext2" || fsType == "ext3" {
		if out, err := exec.Command("e2fsck", "-nf", destPath).CombinedOutput(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() > 1 {
				os.Remove(destPath)
				return fmt.Errorf("verification failed: %s", string(out))
			}
		}
	} else if fsType == "xfs" {
		// xfs_repair -n is the read-only check
		_ = exec.Command("xfs_repair", "-n", destPath).Run()
	}
	fmt.Printf("Filesystem: %s\n", fsType)

	info, _ := os.Stat(destPath)
	fmt.Printf("Image %s ready (%d MB)\n", name, info.Size()/1024/1024)
	return nil
}

// extractRootPartition finds the Linux root partition in a raw disk image
// and copies it out as a standalone ext4 file.
func extractRootPartition(rawDisk, destPath string) error {
	out, err := exec.Command("sfdisk", "--json", rawDisk).CombinedOutput()
	if err != nil {
		return fmt.Errorf("sfdisk: %s: %w", string(out), err)
	}

	offset, size, err := findRootPartition(out)
	if err != nil {
		return err
	}

	cmd := exec.Command("dd",
		fmt.Sprintf("if=%s", rawDisk),
		fmt.Sprintf("of=%s", destPath),
		"bs=512",
		fmt.Sprintf("skip=%d", offset),
		fmt.Sprintf("count=%d", size),
		"status=progress",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("dd extract: %w", err)
	}

	return nil
}

// findRootPartition parses sfdisk JSON output and returns the offset and size
// (in 512-byte sectors) of the Linux root partition.
func findRootPartition(sfdiskJSON []byte) (offset int64, size int64, err error) {
	var result struct {
		PartitionTable struct {
			Partitions []struct {
				Start int64  `json:"start"`
				Size  int64  `json:"size"`
				Type  string `json:"type"`
			} `json:"partitions"`
		} `json:"partitiontable"`
	}

	if err := json.Unmarshal(sfdiskJSON, &result); err != nil {
		return 0, 0, fmt.Errorf("parse sfdisk output: %w", err)
	}

	// GPT partition type GUIDs for Linux root/filesystem
	linuxTypes := map[string]bool{
		"0FC63DAF-8483-4772-8E79-3D69D8477DE4": true, // Linux filesystem
		"4F68BCE3-E8CD-4DB1-96E7-FBCAF984B709": true, // Linux root (x86-64)
		"69DAD710-2CE4-4E3C-B16C-21A1D49ABED3": true, // Linux root (ARM 64)
		"44479540-F297-41B2-9AF7-D131D5F0458A": true, // Linux root (x86)
		"83": true, // MBR Linux
	}

	var bestStart, bestSize int64
	for _, p := range result.PartitionTable.Partitions {
		if linuxTypes[p.Type] && p.Size > bestSize {
			bestStart = p.Start
			bestSize = p.Size
		}
	}

	if bestSize == 0 {
		return 0, 0, fmt.Errorf("no Linux root partition found in disk image")
	}

	return bestStart, bestSize, nil
}

// patchRootfsForFirecracker applies minimal Firecracker-specific fixes to a cloud image.
// With DHCP + cloud-init handling networking/SSH, only fstab cleanup is needed.
func patchRootfsForFirecracker(ext4Path string) error {
	mountDir, err := os.MkdirTemp("", "fcm-patch-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(mountDir)

	if out, err := exec.Command("mount", "-o", "loop", ext4Path, mountDir).CombinedOutput(); err != nil {
		return fmt.Errorf("mount: %s: %w", string(out), err)
	}
	defer func() { _ = exec.Command("umount", mountDir).Run() }()

	// Enable PermitRootLogin — must be in image, not cloud-init runcmd,
	// because sshd starts before runcmd executes.
	if _, err := os.Stat(filepath.Join(mountDir, "etc", "ssh")); err == nil {
		sshdConfDir := filepath.Join(mountDir, "etc", "ssh", "sshd_config.d")
		_ = os.MkdirAll(sshdConfDir, 0755)
		_ = os.WriteFile(filepath.Join(sshdConfDir, "99-fcm.conf"), []byte("PermitRootLogin yes\nPasswordAuthentication yes\n"), 0644)
	}

	// Fix fstab: we extracted just the root partition from a full disk image.
	// Keep only the root (/) entry and comments. Works across all distros.
	fstabPath := filepath.Join(mountDir, "etc", "fstab")
	if data, err := os.ReadFile(fstabPath); err == nil {
		var lines []string
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				lines = append(lines, line) // keep blanks and comments
				continue
			}
			fields := strings.Fields(trimmed)
			if len(fields) >= 2 && fields[1] == "/" {
				lines = append(lines, line) // keep root mount
			}
			// drop everything else (boot, efi, swap, etc.)
		}
		_ = os.WriteFile(fstabPath, []byte(strings.Join(lines, "\n")), 0644)
	}

	if out, err := exec.Command("umount", mountDir).CombinedOutput(); err != nil {
		return fmt.Errorf("umount: %s: %w", string(out), err)
	}

	return nil
}

// detectFS reads the first bytes of a file to determine the filesystem type.
func detectFS(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return "unknown"
	}
	defer f.Close()

	buf := make([]byte, 65536)
	_, _ = f.Read(buf)

	// ext2/3/4: magic 0xEF53 at offset 0x438
	if len(buf) > 0x43A && buf[0x438] == 0x53 && buf[0x439] == 0xEF {
		return "ext4"
	}
	// XFS: magic "XFSB" at offset 0
	if len(buf) > 4 && string(buf[0:4]) == "XFSB" {
		return "xfs"
	}
	return "unknown"
}

func checkPullDeps() error {
	deps := []string{"qemu-img", "sfdisk", "e2fsck"}
	var missing []string
	for _, dep := range deps {
		if _, err := exec.LookPath(dep); err != nil {
			missing = append(missing, dep)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required tools: %s\nInstall with: sudo apt-get install qemu-utils fdisk e2fsprogs",
			strings.Join(missing, ", "))
	}
	return nil
}

func cleanup(paths ...string) {
	for _, p := range paths {
		os.Remove(p)
	}
}

// Import copies a local ext4 file into the images directory.
func Import(name, srcPath string) error {
	if err := os.MkdirAll(imagesDir, 0700); err != nil {
		return fmt.Errorf("create images dir: %w", err)
	}

	destPath := ImagePath(name)
	if err := copyFile(srcPath, destPath); err != nil {
		return fmt.Errorf("import image: %w", err)
	}
	return nil
}

// Remove deletes a cached image.
func Remove(name string) error {
	path := ImagePath(name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("image %q not found", name)
	}
	return os.Remove(path)
}

// CopyForVM copies an image to a VM's rootfs, then resizes it.
func CopyForVM(imageName, destPath string, diskGB int) error {
	srcPath := ImagePath(imageName)
	if _, err := os.Stat(srcPath); err != nil {
		return fmt.Errorf("image %q not found: %w", imageName, err)
	}

	if err := copyReflink(srcPath, destPath); err != nil {
		return fmt.Errorf("copy image: %w", err)
	}

	sizeBytes := fmt.Sprintf("%dG", diskGB)
	if err := exec.Command("truncate", "-s", sizeBytes, destPath).Run(); err != nil {
		return fmt.Errorf("truncate rootfs: %w", err)
	}

	// Resize filesystem to fill the disk
	fsType := detectFS(destPath)
	switch fsType {
	case "ext4", "ext2", "ext3":
		if out, err := exec.Command("e2fsck", "-fy", destPath).CombinedOutput(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() > 1 {
				return fmt.Errorf("e2fsck: %s: %w", string(out), err)
			}
		}
		if out, err := exec.Command("resize2fs", destPath).CombinedOutput(); err != nil {
			return fmt.Errorf("resize2fs: %s: %w", string(out), err)
		}
	case "xfs":
		// XFS can only be resized while mounted
		tmpMount, _ := os.MkdirTemp("", "fcm-xfs-resize-")
		defer os.RemoveAll(tmpMount)
		if out, err := exec.Command("mount", "-o", "loop", destPath, tmpMount).CombinedOutput(); err != nil {
			return fmt.Errorf("mount xfs for resize: %s: %w", string(out), err)
		}
		_ = exec.Command("xfs_growfs", tmpMount).Run()
		_ = exec.Command("umount", tmpMount).Run()
	}

	return nil
}

func resolveImageURL(name string) (string, error) {
	imageURLs := map[string]string{
		// Ubuntu
		"ubuntu":       "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img",
		"ubuntu-24.04": "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img",
		"ubuntu-22.04": "https://cloud-images.ubuntu.com/jammy/current/jammy-server-cloudimg-amd64.img",
		// Debian
		"debian":    "https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-generic-amd64.qcow2",
		"debian-12": "https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-generic-amd64.qcow2",
		// Fedora
		"fedora":    "https://download.fedoraproject.org/pub/fedora/linux/releases/41/Cloud/x86_64/images/Fedora-Cloud-Base-Generic-41-1.4.x86_64.qcow2",
		"fedora-41": "https://download.fedoraproject.org/pub/fedora/linux/releases/41/Cloud/x86_64/images/Fedora-Cloud-Base-Generic-41-1.4.x86_64.qcow2",
		// RHEL family
		"rocky":          "https://download.rockylinux.org/pub/rocky/9/images/x86_64/Rocky-9-GenericCloud-Base.latest.x86_64.qcow2",
		"rocky-9":        "https://download.rockylinux.org/pub/rocky/9/images/x86_64/Rocky-9-GenericCloud-Base.latest.x86_64.qcow2",
		"alma":           "https://repo.almalinux.org/almalinux/9/cloud/x86_64/images/AlmaLinux-9-GenericCloud-latest.x86_64.qcow2",
		"alma-9":         "https://repo.almalinux.org/almalinux/9/cloud/x86_64/images/AlmaLinux-9-GenericCloud-latest.x86_64.qcow2",
		"centos":         "https://cloud.centos.org/centos/9-stream/x86_64/images/CentOS-Stream-GenericCloud-9-latest.x86_64.qcow2",
		"centos-stream9": "https://cloud.centos.org/centos/9-stream/x86_64/images/CentOS-Stream-GenericCloud-9-latest.x86_64.qcow2",
		// Alpine (uses tiny-cloud, partial cloud-init compat)
		"alpine":    "https://dl-cdn.alpinelinux.org/alpine/v3.20/releases/cloud/nocloud_alpine-3.20.0-x86_64-bios-cloudinit-r0.qcow2",
		"alpine-3.20": "https://dl-cdn.alpinelinux.org/alpine/v3.20/releases/cloud/nocloud_alpine-3.20.0-x86_64-bios-cloudinit-r0.qcow2",
		// Arch
		"arch": "https://geo.mirror.pkgbuild.com/images/latest/Arch-Linux-x86_64-cloudimg.qcow2",
		// openSUSE
		"opensuse":      "https://download.opensuse.org/distribution/leap/15.6/appliances/openSUSE-Leap-15.6-Minimal-VM.x86_64-Cloud.qcow2",
		"opensuse-15.6": "https://download.opensuse.org/distribution/leap/15.6/appliances/openSUSE-Leap-15.6-Minimal-VM.x86_64-Cloud.qcow2",
	}

	if url, ok := imageURLs[name]; ok {
		return url, nil
	}

	return "", fmt.Errorf("unknown image %q (available: %s)", name, strings.Join(availableImages(), ", "))
}

func availableImages() []string {
	return []string{
		"ubuntu", "ubuntu-24.04", "ubuntu-22.04",
		"debian", "debian-12",
		"fedora", "fedora-41",
		"rocky", "rocky-9",
		"alma", "alma-9",
		"centos", "centos-stream9",
		"alpine", "alpine-3.20",
		"arch",
		"opensuse", "opensuse-15.6",
	}
}

func downloadFile(url, destPath string) error {
	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("http %d: %s", resp.StatusCode, url)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	written, err := io.Copy(f, resp.Body)
	if err != nil {
		return err
	}

	fmt.Printf("Downloaded %d MB\n", written/1024/1024)
	return nil
}

func copyReflink(src, dst string) error {
	cmd := exec.Command("cp", "--reflink=auto", src, dst)
	if err := cmd.Run(); err != nil {
		return copyFile(src, dst)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

// githubRelease is used for parsing GitHub API responses.
type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// CheckGitHubRelease checks for the latest release of a GitHub repo.
func CheckGitHubRelease(repo string) (*githubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("check release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("github api: %d", resp.StatusCode)
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("parse release: %w", err)
	}
	return &rel, nil
}
