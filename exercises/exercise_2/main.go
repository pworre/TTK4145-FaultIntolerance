package main

import (
	"fmt"
	"log"
	"net"
)

func handleConnection(conn net.Conn) {
	defer conn.Close()
	// Read from client
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		log.Fatalln(err)
	}

	fmt.Println(string(buf[:n]))
}

func udp_listen() {
	ln, err := net.Listen("udp", "localhost:30000")
	if err != nil {
		log.Fatalln(err)
	}

	// close listen-line when finished
	defer ln.Close()

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Fatalln(err)
		}

		go handleConnection(conn)
	}
}

func main() {
	//udp_listen()

	// UDP address
	addr, err := net.ResolveUDPAddr("udp", ":30000")
	if err != nil {
		log.Fatal("Could not resolve address:")
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatal("Listen failed:", err)
	}
	defer conn.Close()

	buf := make([]byte, 1024)
	for {
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Println(err)
			continue
		}
		fmt.Printf("Received msg from %s: %s\n", remoteAddr, string(buf[:n]))
	}

	//net.DialUDP()
}