package main

import (
	"fmt"
	"log"
	"net"
	"time"
)

/*
type MsgFormat int

const (
	PortVariable = 33546
	PortFixed    = 34933
)

type TCPConfig struct {
	ServerIP string
	ServerPort int
	Message string
	TimeOut time.Duration
}


func TCP_SendAndReceive(listenSocket *net.TCPListener) {
	addr := listenSocket.Addr()
	serverAddr := &net.TCPAddr{
		IP:   net.ParseIP("10.100.23.11"),
		Port: 20020,
		//Zone: "",
	}
	conn, err := net.Accept("tcp", serverAddr, addr)
	if err != nil {
		fmt.Printf("Could not connect: %w", err)
	}
	defer conn.Close()
	msg := []byte("From server to local")
	n, err := conn.Write(msg)
	if err != nil {
		log.Fatal("Write error: ")
	}
	buf := make([]byte, 1024)
	n, err = conn.Read(buf)
	if err != nil {
		log.Fatal("Read error:")
	}
	// print from buffer
	fmt.Printf("Received msg from %s: %s\n", addr, string(buf[:n]))
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
		return "", fmt.Errorf("TCP Read error: %w", err)
	}

	return string(buf[:n]), nil
}

func TCP_listen(localIP string, localPort int) {
	addr := fmt.Sprintf("%s:%d", localIP, localPort)
	// Create socket and binds it to IP-address + PORT. Sets socket to LISTEN-state
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("Listen error: %v", err)
	}
	defer ln.Close()
	log.Printf("STATUS: Listening on address %s", addr)

	// SERVER running. TCP-stream established
	for {
		// OS checks acceptance-queue for new TCP-msg
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("TCP acceptance-error: %v", err)
		}


		// Go-routine for each client
		go func(c net.Conn) {
			defer c.Close()
			buf := make([]byte, 1024)
			// Wait for client to send data
			n_bytes, err := c.Read(buf)
			if err != nil {
				log.Println("TCP: Could not read: %w", err)
				return
			}
			fmt.Printf("Received data: %s", string(buf[:n_bytes]))

			// Echo
			_, err = c.Write([]byte(string(buf[:n_bytes]) + "\x00"))
			if err != nil {
				log.Printf("write error: %v", err)
			}
		}(conn)
	}
}
*/

func TCP_Client(serverPort int) {
	// Connection-request to server
	serverAddr := &net.TCPAddr{
		IP:   net.ParseIP("10.100.23.11"),
		Port: serverPort,
		//Zone: "",
	}
	conn, err := net.DialTCP("tcp", nil, serverAddr)
	if err != nil {
		log.Fatal("Could not dial up server:")
	}
	defer conn.Close()
	msg := []byte("Connect to: 10.100.23.30:20020\x00")
	_, err = conn.Write(msg)
	if err != nil {
		log.Fatal("Write error: ")
	}
}

func TCP_Server(localPort int) {
	localAddr := &net.TCPAddr{
		Port: localPort,
	}
	listener, err := net.ListenTCP("tcp", localAddr)
	if err != nil {
		log.Fatal("Listen error:")
	}
	defer listener.Close()
	log.Printf("Listening on port %d\n", localPort)

	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			log.Fatal("Accept error:")
		}
		go handleConnection(conn)
	}
}

func handleConnection(conn *net.TCPConn) {
	defer conn.Close()

	// Receive message from server to GRP18
	buf := make([]byte, 1024)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			log.Fatal("Could not read from server")
		}
		fmt.Printf("Received msg from server! Here is the message, it has been sent from my local pc to the server, been directed back, and here it is received: %s\n", string(buf[:n]))

		// Send message from local pc to server
		msg := []byte("Hello from GRP18 to server!")
		n, err = conn.Write(msg)
		if err != nil {
			log.Fatal("Write error from server to local: ")
		}
	}
}

func main() {
	go TCP_Server(20020)
	time.Sleep(time.Second)
	TCP_Client(34933)

	select {}
}

// 20020 = localport
// 34933 = serverport
