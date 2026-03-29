package network

import (
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/server4"

	"fcm.dev/fcm-cli/internal/config"
	"fcm.dev/fcm-cli/internal/vm"
)

var (
	dhcpServer *server4.Server
	dhcpMu     sync.Mutex
)

// StartDHCP starts an embedded DHCP server on the bridge interface.
// It assigns IPs to VMs based on their MAC address (from vm.json).
func StartDHCP(cfg *config.Config) error {
	dhcpMu.Lock()
	defer dhcpMu.Unlock()

	if dhcpServer != nil {
		return nil // already running
	}

	iface, err := net.InterfaceByName(cfg.BridgeName)
	if err != nil {
		return fmt.Errorf("dhcp: find interface %s: %w", cfg.BridgeName, err)
	}

	handler := &dhcpHandler{cfg: cfg}

	srv, err := server4.NewServer(
		iface.Name,
		nil, // listen on all addresses
		handler.handle,
	)
	if err != nil {
		return fmt.Errorf("dhcp: create server: %w", err)
	}

	dhcpServer = srv

	go func() {
		slog.Info("DHCP server started", "interface", cfg.BridgeName, "subnet", cfg.BridgeSubnet)
		if err := srv.Serve(); err != nil {
			slog.Error("DHCP server error", "error", err)
		}
	}()

	return nil
}

// StopDHCP stops the embedded DHCP server.
func StopDHCP() {
	dhcpMu.Lock()
	defer dhcpMu.Unlock()

	if dhcpServer != nil {
		dhcpServer.Close()
		dhcpServer = nil
	}
}

type dhcpHandler struct {
	cfg *config.Config
}

func (h *dhcpHandler) handle(conn net.PacketConn, peer net.Addr, req *dhcpv4.DHCPv4) {
	if req.OpCode != dhcpv4.OpcodeBootRequest {
		return
	}

	// Look up the VM by MAC address
	vmInfo := h.findVMByMAC(req.ClientHWAddr.String())
	if vmInfo == nil {
		slog.Debug("DHCP: unknown MAC", "mac", req.ClientHWAddr)
		return
	}

	// Build response
	resp, err := dhcpv4.NewReplyFromRequest(req)
	if err != nil {
		slog.Error("DHCP: build reply", "error", err)
		return
	}

	ip := net.ParseIP(vmInfo.IP)
	gateway := net.ParseIP(h.cfg.BridgeIP)
	dns := net.ParseIP(h.cfg.DNS)
	mask := net.ParseIP(h.cfg.BridgeMask)

	resp.YourIPAddr = ip
	resp.ServerIPAddr = gateway
	resp.Options.Update(dhcpv4.OptSubnetMask(net.IPMask(mask.To4())))
	resp.Options.Update(dhcpv4.OptRouter(gateway))
	resp.Options.Update(dhcpv4.OptDNS(dns))
	resp.Options.Update(dhcpv4.OptIPAddressLeaseTime(86400)) // 24 hours
	resp.Options.Update(dhcpv4.OptHostName(vmInfo.Name))

	switch req.MessageType() {
	case dhcpv4.MessageTypeDiscover:
		resp.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeOffer))
	case dhcpv4.MessageTypeRequest:
		resp.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeAck))
	default:
		return
	}

	slog.Info("DHCP", "type", req.MessageType(), "mac", req.ClientHWAddr, "ip", vmInfo.IP, "vm", vmInfo.Name)

	if _, err := conn.WriteTo(resp.ToBytes(), peer); err != nil {
		slog.Error("DHCP: send reply", "error", err)
	}
}

func (h *dhcpHandler) findVMByMAC(mac string) *vm.VM {
	vms, err := vm.LoadAllVMs()
	if err != nil {
		return nil
	}

	for _, v := range vms {
		if v.MAC == mac {
			return v
		}
	}
	return nil
}
