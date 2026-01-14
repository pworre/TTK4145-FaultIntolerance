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
	// Connects to server with a given TimeOut
	conn, err := net.DialTimeout("tcp", addr, cfg.TimeOut)
	if err != nil {
		return "", fmt.Errorf("Could not connect: %w", err)
	}
	defer conn.Close()

	_, err = conn.Write([]byte(cfg.Message))
	if err != nil {
		return "", fmt.Errorf("TCP write-error: %w", err)
	}

	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		return "", fmt.Errorf("TCP Read error: %w", err)
	}

	return string(buf[:n]), nil
}

func TCP_listen(localIP string, localPort int) {
	addr := fmt.Sprintf("%s:%d", localIP, localPort)
	// Create socket and binds it to IP-address + PORT. Sets socket to LISTEN-state
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal("Listen error: %w", err)
	}
	defer ln.Close()
	log.Printf("STATUS: Listening on address %s", addr)

	// SERVER running. TCP-stream established
	for {
		// OS checks acceptance-queue for new TCP-msg
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("TCP acceptance-error: %w", err)
		}
	

		// Go-routine for each client
		go func(c net.Conn) {
			defer c.Close()
			buf := make([]byte, 1024)
			// Wait for client to send data
			n_bytes, err := c.Read(buf)
			if err != nil {
				log.Println("TCP: Could not read: %v", err)
				return
			}
			fmt.Printf("Received data: %s", string(buf[:n_bytes]))
		}(conn)
	}
}

func main() {
	cfg := TCPConfig{
		ServerIP: 	"10.100.23.11",
		ServerPort: FIXED_LENGTH,
		Message:	"Connect to: 10.100.23.30:20020\x00",
		TimeOut: 	5 * time.Second,
	}

	response, err := TCP_SendAndReceive(cfg)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Response: %s", response)
}