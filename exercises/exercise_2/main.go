package main

import (
	"fmt"
	"log"
	"net"
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
		// clear buffer
		buf = buf[:0]

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

func udp_write(msg []byte)
	// Sender
	sendAddr := &net.UDPAddr{
		IP: net.ParseIP("10.100.23.11"), 
		Port: 30000,
		//Zone: "",
	}

	conn, err := net.DialUDP("udp", nil, sendAddr)
	defer conn.Close()

	conn.Write(msg)

func main() {
	// UDP address
	addr, err := net.ResolveUDPAddr("udp", ":30000")
	if err != nil {
		log.Fatal("Could not resolve address:")
	}

	go func(){
		udp_listen(addr)
	}()
	go func(){
		udp_write("Hello World from GRP18\n")
	}
	



}
