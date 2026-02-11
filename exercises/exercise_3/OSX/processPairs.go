package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"time"
)

type State struct {
	Counter   int  `json:"counter"`
	IsPrimary bool `json:"isPrimary"`
}

const (
	broadcastInterval = 1000 * time.Millisecond
	primaryTimeOut    = 3 * broadcastInterval
	port              = 3000
)

// Broadcast 255.255.255.255:<port>
// Sending net.DialUDP
// Receiving net.ListenUDP

func spawnBackup() {
	dir, erro := os.Getwd()
	if erro != nil {
		panic(erro)
	}

	cmd := fmt.Sprintf(`tell app "Terminal" to do script "cd %s; go run processPairs.go"`, dir)

	err := exec.Command(
		"osascript",
		"-e",
		cmd,
	).Run()

	if err != nil {
		panic(err)
	}
}

func main() {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		log.Println("Failed to resolve UDP receive Addr")
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Println("Failed to connect UDP listener")
	}
	defer conn.Close()

	state := State{Counter: 0, IsPrimary: false}
	lastPrimary := time.Now()

	conn.SetReadDeadline(time.Now().Add(broadcastInterval))

	// Backup loop
	buf := make([]byte, 1024)
	for {
		conn.SetReadDeadline(time.Now().Add(broadcastInterval))

		numBytes, _, _ := conn.ReadFromUDP(buf)
		var incoming State
		json.Unmarshal(buf[:numBytes], &incoming)

		if incoming.IsPrimary {
			lastPrimary = time.Now()
			state.Counter = incoming.Counter
		}

		if time.Since(lastPrimary) > primaryTimeOut {
			break
		}
	}

	conn.Close()
	spawnBackup()
	state.IsPrimary = true

	sendAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("255.255.255.255:%d", port))
	if err != nil {
		log.Println("Failed to resolve UDP send adress")
	}

	sendConn, err := net.DialUDP("udp", nil, sendAddr)
	if err != nil {
		log.Println("Failed to dial up UDP")
	}
	defer sendConn.Close()

	// Primary loop
	for {
		state.Counter++
		fmt.Printf("Counter: %d\n", state.Counter)

		data, err := json.Marshal(state)
		if err != nil {
			log.Printf("Failed to make json of state to send")
		}
		sendConn.Write(data)

		time.Sleep(broadcastInterval)
	}
}
