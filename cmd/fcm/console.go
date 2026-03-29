package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"syscall"

	"fcm.dev/fcm-cli/internal/systemd"
	"fcm.dev/fcm-cli/internal/vm"
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
)

var consoleCmd = &cobra.Command{
	Use:   "console [name]",
	Short: "Attach to a VM's serial console",
	Long:  "Interactive serial console via the Firecracker Unix socket. Press Ctrl+] to detach.",
	Args:  cobra.ExactArgs(1),
	RunE:  runConsole,
}

func init() {
	rootCmd.AddCommand(consoleCmd)
}

func runConsole(cmd *cobra.Command, args []string) error {
	if err := requireRoot(); err != nil {
		return err
	}

	name := args[0]
	v, err := vm.LoadVM(name)
	if err != nil {
		return err
	}

	unit := systemd.VMUnitName(name)
	if !systemd.IsActive(unit) {
		return fmt.Errorf("vm %q is not running", name)
	}

	// Connect to the Firecracker Unix socket for serial I/O
	conn, err := net.Dial("unix", v.SocketPath)
	if err != nil {
		return fmt.Errorf("connect to serial: %w", err)
	}
	defer conn.Close()

	fmt.Printf("Connected to %s serial console. Press Ctrl+] to detach.\n", name)

	// Put terminal in raw mode
	oldState, err := makeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("set raw mode: %w", err)
	}
	defer restoreTerminal(int(os.Stdin.Fd()), oldState)

	// Handle signals for clean exit
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		conn.Close()
	}()

	done := make(chan struct{})

	// Copy socket -> stdout
	go func() {
		_, _ = io.Copy(os.Stdout, conn)
		close(done)
	}()

	// Copy stdin -> socket (with Ctrl+] escape)
	go func() {
		buf := make([]byte, 1)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil || n == 0 {
				break
			}
			// Ctrl+] (0x1D) to detach
			if buf[0] == 0x1D {
				fmt.Println("\nDetached from console.")
				conn.Close()
				return
			}
			_, _ = conn.Write(buf[:n])
		}
	}()

	<-done
	return nil
}

func makeRaw(fd int) (*unix.Termios, error) {
	termios, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return nil, err
	}

	oldState := *termios

	// Set raw mode
	termios.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP | unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	termios.Oflag &^= unix.OPOST
	termios.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	termios.Cflag &^= unix.CSIZE | unix.PARENB
	termios.Cflag |= unix.CS8
	termios.Cc[unix.VMIN] = 1
	termios.Cc[unix.VTIME] = 0

	if err := unix.IoctlSetTermios(fd, unix.TCSETS, termios); err != nil {
		return nil, err
	}

	return &oldState, nil
}

func restoreTerminal(fd int, state *unix.Termios) {
	_ = unix.IoctlSetTermios(fd, unix.TCSETS, state)
}
