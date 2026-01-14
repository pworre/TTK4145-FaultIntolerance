package main

import (
	"fmt"
	"log"
	"net"
	"sync"
)

func udp_listen(addr *net.UDPAddr) {
	// Create and bind socket
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatal("Listen failed:", err)
	}
	defer conn.Close()

	// Buffer to hold received information
	buf := make([]byte, 1024)
	for {
		// listen to connection
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Println(err)
			continue
		}

		// print from buffer
		fmt.Printf("Received msg from %s: %s\n", remoteAddr, string(buf[:n]))
	}
}

func udp_write(msg []byte) {
	// Sender (11)
	sendAddr := &net.UDPAddr{
		IP:   net.ParseIP("10.100.23.255"),
		Port: 20020,
		//Zone: "",
	}

	conn, err := net.DialUDP("udp", nil, sendAddr)
	if err != nil {
		log.Fatal("Could not dial up server:")
	}
	defer conn.Close()

	conn.Write(msg)
}

func main() {
	// UDP address
	addr, err := net.ResolveUDPAddr("udp", ":20020")
	if err != nil {
		log.Fatal("Could not resolve address:")
	}

	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		udp_listen(addr)
	}()
	go func() {
		defer wg.Done()
		msg := []byte("Hello World from GRP18!\n")
		udp_write(msg)
	}()
	wg.Wait()
}
