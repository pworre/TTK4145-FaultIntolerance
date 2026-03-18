package processPairs

// - - - - - - Overview - - - - - - - - -

// This module contains logic for maintaining a process pairs configuration of a main.go program.
// Upon the closing of a primary, this thread will spawn a new backup before itself transitions to primary,
// and signal to the main backup process that it can take over as a primary

import (
	"elevator_project/config"
	"elevator_project/syncOrders"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"time"
)

type State struct {
	CurrentWorldView syncOrders.WorldView `json:"worldView"`
	IsPrimary        bool                 `json:"isPrimary"`
	Restart          bool                 `json:"restart"`
}

const (
	BROADCAST_INTERVAL   = 50 * time.Millisecond
	PRIMARY_TIMEOUT      = 6 * BROADCAST_INTERVAL
	PROCESSPAIR_BASEPORT = 3000
)

func spawnBackup(cfg config.Config) error {
	port := cfg.Port
	id := cfg.ID

	dir, err := os.Getwd()
	if err != nil {
		return err
	}

	switch runtime.GOOS {
	case "darwin":
		cmdStr := fmt.Sprintf(
			`tell application "Terminal" to do script "cd '%s'; go run . -id=%s -port=%d -backup"`,
			dir, id, port,
		)
		return exec.Command("osascript", "-e", cmdStr).Run()

	case "linux":
		if _, err := exec.LookPath("gnome-terminal"); err == nil {
			cmd := exec.Command(
				"gnome-terminal",
				"--",
				"bash",
				"-c",
				fmt.Sprintf("cd %q && exec go run main.go -id=%q -port=%d -backup; exit", dir, id, port),
			)
			return cmd.Start()
		}

		// Fallback
		if _, err := exec.LookPath("x-terminal-emulator"); err == nil {
			cmd := exec.Command(
				"x-terminal-emulator",
				"-e",
				"bash",
				"-c",
				fmt.Sprintf("cd %q && execgo run main.go -id=%q -port=%d -backup; exit", dir, id, port),
			)
			return cmd.Start()
		}

		return fmt.Errorf("No supperted terminal for linux")

	case "windows":
		return fmt.Errorf("Windows not implemented")

	default:
		return fmt.Errorf("OS not supported")
	}
}

func RunProcessPairs(cfg config.Config, primaryUpdate <-chan syncOrders.WorldView, restart chan bool, takeOver chan syncOrders.WorldView) {

	peerID_int, err := strconv.Atoi(cfg.ID)
	if err != nil {
		log.Printf("invalid cfg.ID %q: %v", cfg.ID, err)
		return
	}
	processPairPort := PROCESSPAIR_BASEPORT + peerID_int

	log.Printf("ProcessPairPort: %d", processPairPort)

	state := State{IsPrimary: false}
	lastPrimary := time.Now()

	if cfg.Backup {
		addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.2:%d", processPairPort))
		if err != nil {
			log.Println("Failed to resolve UDP receive Addr")
		}

		conn, err := net.ListenUDP("udp", addr)
		if err != nil {
			log.Println("Failed to connect UDP listener:", err)
			return
		}

		// Backup loop
		buf := make([]byte, 1024)
		takeOver := false
		for {
			conn.SetReadDeadline(time.Now().Add(BROADCAST_INTERVAL))

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

					if incoming.Restart {
						log.Println("Primary requested restart!")
						takeOver = true
					}
				}
			}

			if takeOver || time.Since(lastPrimary) > PRIMARY_TIMEOUT {
				break
			}
		}
		conn.Close()
	}

	state.IsPrimary = true

	if state.CurrentWorldView.PeerID == "" {
		log.Println("Trying to restore empty id")
		state.CurrentWorldView.PeerID = cfg.ID
	}

	if err := spawnBackup(cfg); err != nil {
		log.Println("Failed to spawn backup:", err)
	}

	log.Printf("Sending restored worldview with cab orders: %+v", state.CurrentWorldView)
	takeOver <- state.CurrentWorldView

	sendAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.2:%d", processPairPort))
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
		case latestWorldView := <-primaryUpdate:
			state.CurrentWorldView = latestWorldView

		case <-restart:
			state.Restart = true

			// Send one last state and end
			data, err := json.Marshal(state)
			if err == nil {
				_, err = sendConn.Write(data)
				if err != nil {
					log.Println("Failed to send restart state:", err)
				}
			}

			log.Println("Primary requested restart!")
			os.Exit(0)

		default:
		}

		data, err := json.Marshal(state)
		if err != nil {
			log.Printf("Failed to make json of state to send")
			time.Sleep(BROADCAST_INTERVAL)
			continue
		}

		_, err = sendConn.Write(data)
		if err != nil {
			log.Println("Failed to send state:", err)
		}

		time.Sleep(BROADCAST_INTERVAL)
	}
}
