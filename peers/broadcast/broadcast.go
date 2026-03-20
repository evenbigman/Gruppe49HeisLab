package broadcast

import (
	"context"
	"fmt"
	"github.com/vmihailenco/msgpack/v5"
	"log"
	"net"
	"sanntidslab/peers/snapshots"
	"syscall"
)

const bufferSize = 1024

type Msg_t struct {
	Sender    uint64
	Snapshots map[uint64]snapshots.Snapshot_t
}

func Receiver(port int, rx chan Msg_t) {
	cfg := net.ListenConfig{
		Control: broadcastControl,
	}

	conn, err := cfg.ListenPacket(context.Background(), "udp4", fmt.Sprintf(":%d", port))
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	//log.Printf("[Receiver] Started on port: %d", port)

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

		//log.Println(msg)
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

	conn, err := cfg.ListenPacket(context.Background(), "udp4", fmt.Sprintf(":%d", 0))
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	log.Printf("[Transmitter] Started on port: %d", port)

	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("255.255.255.255:%d", port))
	if err != nil {
		panic(err)
	}

	for msg := range tx {
		data, err := msgpack.Marshal(msg)
		if err != nil {
			log.Printf("[Transmitter] Encoding error: %s", err)
			continue
		}
		_, err = conn.WriteTo(data, addr)
		if err != nil {
			log.Printf("[Transmitter] Sending error: %s", err)
			continue
		}

		//log.Printf("[Transmitter] Sending %d bytes", n)
	}
}

func broadcastControl(network, address string, c syscall.RawConn) error {
	return c.Control(func(fd uintptr) {
		err := syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
		if err != nil {
			log.Printf("SO_REUSEADDR error: %s", err)
		}
		err = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
		if err != nil {
			log.Printf("SO_BROADCAST error: %s", err)
		}
	})
}
