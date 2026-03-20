package broadcast

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"sanntidslab/peers/snapshots"
	"strings"

	"github.com/vmihailenco/msgpack/v5"
)

const bufferSize = 1024

type Msg_t struct {
	Sender    uint64
	Snapshots map[uint64]snapshots.Snapshot
}

func Receiver(port int, rx chan Msg_t) {
	cfg := net.ListenConfig{
		Control: broadcastControl,
	}

	conn, err := cfg.ListenPacket(context.Background(), "udp4", net.JoinHostPort(recvIP, fmt.Sprintf("%d", port)))
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	log.Printf("[Receiver] Listening on %s", conn.LocalAddr().String())

	buffer := make([]byte, bufferSize)

	for {
		n, _, err := conn.ReadFrom(buffer)
		if err != nil {
			log.Printf("[Receiver] Reading error: %s", err)
			continue
		}

		msg := Msg_t{}

		err = msgpack.Unmarshal(buffer[:n], &msg)
		if err != nil {
			log.Printf("[Receiver] Decoding error: %s", err)
			continue
		}

		rx <- msg
		//if udpAddr, ok := addr.(*net.UDPAddr); ok{
		//log.Printf("[Receiver] Received %d bytes from %s at port %d", n, udpAddr.IP, udpAddr.Port)
		//}
	}
}

func Transmitter(port int, tx chan Msg_t) {
	cfg := net.ListenConfig{
		Control: broadcastControl,
	}

	conn, err := cfg.ListenPacket(context.Background(), "udp4", bindAddr)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	log.Printf("[Transmitter] Started on port: %d", port)
	log.Printf("[Transmitter] Local socket: %s", conn.LocalAddr().String())

	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", dstIP.String(), port))
	if err != nil {
		panic(err)
	}

	log.Printf("[Transmitter] Destination: %s", addr.String())

	for msg := range tx {
		data, err := msgpack.Marshal(msg)
		if err != nil {
			log.Printf("[Transmitter] Encoding error: %s", err)
			continue
		}
		n, err := conn.WriteTo(data, addr)
		if err != nil {
			log.Printf("[Transmitter] Sending error: %s", err)
			continue
		}

		log.Printf("[Transmitter] Sent %d bytes to %s", n, addr.String())
	}
}

func resolveBroadcastTarget() (net.IP, net.IP, error) {
	var srcIP net.IP
	var mask net.IPMask
	var err error

	ifaceOverride := os.Getenv("BCAST_IFACE")
	srcOverride := os.Getenv("BCAST_SRC")
	dstOverride := os.Getenv("BCAST_ADDR")

	switch {
	case ifaceOverride != "":
		srcIP, mask, err = firstIPv4ForInterface(ifaceOverride)
	case srcOverride != "":
		srcIP, mask, err = findIPv4ByAddress(srcOverride)
	default:
		srcIP, mask, err = firstUsableIPv4()
	}
	if err != nil {
		return nil, nil, err
	}

	if dstOverride != "" {
		dstIP := net.ParseIP(dstOverride).To4()
		if dstIP == nil {
			return nil, nil, fmt.Errorf("invalid BCAST_ADDR: %s", dstOverride)
		}
		return srcIP, dstIP, nil
	}

	return srcIP, directedBroadcast(srcIP, mask), nil
}

func firstUsableIPv4() (net.IP, net.IPMask, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, nil, err
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if isLikelyVirtualInterface(iface.Name) {
			continue
		}

		ip, mask, err := firstIPv4ForInterface(iface.Name)
		if err == nil {
			return ip, mask, nil
		}
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		ip, mask, err := firstIPv4ForInterface(iface.Name)
		if err == nil {
			return ip, mask, nil
		}
	}

	return nil, nil, errors.New("no usable IPv4 interface found; set BCAST_IFACE or BCAST_SRC")
}

func firstIPv4ForInterface(interfaceName string) (net.IP, net.IPMask, error) {
	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		return nil, nil, err
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return nil, nil, err
	}

	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		ip4 := ipNet.IP.To4()
		if ip4 == nil {
			continue
		}
		return ip4, ipNet.Mask, nil
	}

	return nil, nil, fmt.Errorf("interface %s has no IPv4 address", interfaceName)
}

func findIPv4ByAddress(ipString string) (net.IP, net.IPMask, error) {
	target := net.ParseIP(ipString).To4()
	if target == nil {
		return nil, nil, fmt.Errorf("invalid BCAST_SRC: %s", ipString)
	}

	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, nil, err
	}

	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipNet.IP.To4()
			if ip4 == nil {
				continue
			}
			if ip4.Equal(target) {
				return ip4, ipNet.Mask, nil
			}
		}
	}

	return nil, nil, fmt.Errorf("BCAST_SRC %s not found on any interface", ipString)
}

func directedBroadcast(ip net.IP, mask net.IPMask) net.IP {
	out := make(net.IP, len(ip))
	for i := 0; i < len(ip); i++ {
		out[i] = ip[i] | ^mask[i]
	}
	return out
}

func isLikelyVirtualInterface(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "virtual") ||
		strings.Contains(lower, "vethernet") ||
		strings.Contains(lower, "hyper-v") ||
		strings.Contains(lower, "wsl") ||
		strings.Contains(lower, "loopback")
}
