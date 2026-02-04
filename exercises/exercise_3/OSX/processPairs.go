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
	counter   int	`json:"counter"`
	isPrimary bool	`json:"isPrimary`
}

const (
	broadcastIntervall = 100 * time.Millisecond
	primaryTimeOut     = 500 * time.Millisecond
	port               = 3000
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

	sendAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("255.255.255.255:%d", port))
	if err != nil {
		log.Println("Failed to resolve UDP send adress")
	}

	sendConn, err := net.DialUDP("udp", nil, sendAddr)
	if err != nil {
		log.Println("Failed to listen UDP")
	}

	state := State{counter: 0, isPrimary: false}
	lastPrimary := time.Now()

	// RECEIVER
	go func() {
		buf := make([]byte, 1024)
		for {
			numBytes, _, _ := conn.ReadFromUDP(buf)
			var incoming State

			json.Unmarshal(buf[:numBytes], &incoming)

			if incoming.isPrimary {
				lastPrimary = time.Now()
				if !state.isPrimary {
					state.counter = incoming.counter
				}
			}
		}
	}()

	ticker := time.NewTicker(1 * time.Second)
	isBackupSpawned := false

	for {
		select {
		case <-ticker.C:
			if state.isPrimary {
				state.counter++
				fmt.Printf("Counter: %d\n", state.counter)
			}

			// Backup becomes primary if timeout
			if time.Since(lastPrimary) > primaryTimeOut {
				state.isPrimary = true
				if !isBackupSpawned {
					spawnBackup()
					isBackupSpawned = true
				}

			}

			data, err := json.Marshal(state)
			if err != nil {
				log.Printf("Failed to make json of state to send")
			}
			sendConn.Write(data)
		}
	}
}
