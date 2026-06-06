package proxy

/*

import (
	"io"
	"net"
)

type MQTTProxy struct {
	targets []string
}

func (m *MQTTProxy) Listen(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go m.handleMQTTConn(conn)
	}
}

func (m *MQTTProxy) handleMQTTConn(client net.Conn) {
	defer client.Close()

	backend, err := net.Dial("tcp", m.pickTarget(client.RemoteAddr().String()))
	if err != nil {
		return
	}
	defer backend.Close()

	go io.Copy(backend, client)
	io.Copy(client, backend)
}

func (m *MQTTProxy) pickTarget() {

}
*/
