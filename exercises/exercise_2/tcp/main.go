package main

import (
	"fmt"
	"log"
	"net"
	"time"
)

type MsgFormat int

const (
	VARIABLE_LENGTH = 33546
	FIXED_LENGTH    = 34933
)

type TCPConfig struct {
	ServerIP string
	ServerPort int
	Message string
	TimeOut time.Duration
}

func TCP_SendAndReceive(cfg TCPConfig) (string, error) {
	addr := fmt.Sprintf("%s:%d", cfg.ServerIP, cfg.ServerPort)
	conn, err := net.DialTimeout("tcp", addr, cfg.TimeOut)
	if err != nil {
		fmt.Printf("Could not connect: %w", err)
	}

	defer conn.Close()

	

}

func TCP_Client(format MsgFormat) {
	// Connection-request to server
	serverAddr := &net.TCPAddr{
		IP:   net.ParseIP("10.100.23.11"),
		Port: int(format),
		//Zone: "",
	}
	conn, err := net.DialTCP("tcp", nil, serverAddr)
	if err != nil {
		log.Fatal("Could not dial up server:")
	}
	defer conn.Close()
	msg := []byte("Connect to: 10.100.23.30:20020\x00")
	n, err := conn.Write(msg)
	if err != nil {
		log.Fatal("Write error: ")
	}

	// Local port
	localAddr := &net.TCPAddr{
		IP:   net.ParseIP("10.100.23.30"),
		Port: 20020,
		//Zone: "",
	}
	ln, err = net.ListenTCP("tcp", localAddr)
	if err != nil {
		log.Fatal("Dial error:")
	}
	buf := make([]byte, 1024)
	n, err = ln.Read(buf)
	if err != nil {
		log.Fatal("Read error:")
	}
	// print from buffer
	fmt.Printf("Received msg from %s: %s\n", localAddr, string(buf[:n]))
}

func TCP_read(localPort int) {
	addr := &net.TCPAddr{
		IP:   net.ParseIP("10.100.23.11"),
		Port: int(localPort),
		//Zone: "",

	}
	conn, err := net.DialTCP("tcp", nil, addr)
	if err != nil {
		log.Fatal("Could not dial up server:")
	}
	defer conn.Close()

	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		log.Fatal("Read error:")
	}
	// print from buffer
	fmt.Printf("Received msg from %s: %s\n", addr, string(buf[:n]))
}

func main() {
	TCP_Client(FIXED_LENGTH)
}
