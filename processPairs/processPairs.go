package processPairs

import (
	"elevator_project/syncOrders"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"runtime"
	"time"
)

type State struct {
	CurrentWorldView syncOrders.WorldView `json:"worldView"`
	IsPrimary        bool                 `json:"isPrimary"`
}

const (
	broadcastInterval = 50 * time.Millisecond
	primaryTimeOut    = 3 * broadcastInterval
	port              = 3000
)

// Broadcast 255.255.255.255:<port>
// Sending net.DialUDP
// Receiving net.ListenUDP

func spawnBackup(peerID string) {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	switch runtime.GOOS {
	case "darwin":
		cmdStr := fmt.Sprintf(`tell app "Terminal" to do script "cd %s; go run main.go --id=%s"`, dir, peerID)
		err = exec.Command("osascript", "-e", cmdStr).Run()
	case "linux":
		cmd := exec.Command("go", "run", "main.go", "--id="+peerID)
		cmd.Dir = dir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		err = cmd.Start()

	case "windows":
		err = fmt.Errorf("windows not implemented")
	default:
		err = fmt.Errorf("OS not supported")
	}

	if err != nil {
		panic(err)
	}
}

func RunProcessPairs(worldViewCh <-chan syncOrders.WorldView) {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		log.Println("Failed to resolve UDP receive Addr")
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Println("Failed to connect UDP listener")
	}
	defer conn.Close()

	state := State{IsPrimary: false}
	lastPrimary := time.Now()

	// Backup loop
	buf := make([]byte, 1024)
	for {
		conn.SetReadDeadline(time.Now().Add(broadcastInterval))

		select {
		case latestWorldView := <-worldViewCh:
			state.CurrentWorldView = latestWorldView
		default:
		}

		numBytes, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, valid := err.(net.Error); valid && netErr.Timeout() {

			} else {
				log.Println("ReadFromUDP failed:", err)
			}
		} else {
			var incoming State

			err := json.Unmarshal(buf[:numBytes], &incoming)
			if err != nil {
				log.Println("Unmarshal failed:", err)
			} else if incoming.IsPrimary {
				lastPrimary = time.Now()
				state.CurrentWorldView = incoming.CurrentWorldView
			}
		}

		if time.Since(lastPrimary) > primaryTimeOut {
			break
		}
	}
	conn.Close()

	spawnBackup(state.CurrentWorldView.PeerID)
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
		select {
		case latestWorldView := <-worldViewCh:
			state.CurrentWorldView = latestWorldView
		default:
		}

		data, err := json.Marshal(state)
		if err != nil {
			log.Printf("Failed to make json of state to send")
			continue
		}
		sendConn.Write(data)
		time.Sleep(broadcastInterval)

	}
}
