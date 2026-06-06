package tcpproxy

import (
	"fmt"
	"io"
	"log"
	"net"
	"sync"
)

type MQTTProxy struct {
	port    int
	targets []string
	mu      sync.Mutex
	counter uint64
}

func NewMQTTProxy(port int, targets []string) *MQTTProxy {
	return &MQTTProxy{
		port:    port,
		targets: targets,
	}
}

func (m *MQTTProxy) Listen() error {
	addr := fmt.Sprintf(":%d", m.port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to start MQTT proxy on port %d: %w", m.port, err)
	}

	log.Printf("MQTT proxy (layer 4) active on port %d", m.port)

	for {
		clientConn, err := ln.Accept()
		if err != nil {
			log.Printf("error accepting MQTT connection: %v", err)
			continue
		}

		go m.handleMQTTConn(clientConn)
	}
}

func (m *MQTTProxy) handleMQTTConn(client net.Conn) {
	defer client.Close()

	targetAddr := m.pickTarget()
	if targetAddr == "" {
		log.Println("no MQTT targets configured")
		return
	}

	backend, err := net.Dial("tcp", targetAddr)
	if err != nil {
		log.Printf("MQTT proxy failed to connect to broker %s: %v", targetAddr, err)
		return
	}
	defer backend.Close()

	errc := make(chan error, 2)

	go func() {
		_, err := io.Copy(backend, client)
		errc <- err
	}()

	go func() {
		_, err := io.Copy(client, backend)
		errc <- err
	}()

	<-errc
}

func (m *MQTTProxy) pickTarget() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.targets) == 0 {
		return ""
	}

	target := m.targets[m.counter%uint64(len(m.targets))]
	m.counter++
	return target
}
