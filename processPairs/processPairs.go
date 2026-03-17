package processPairs

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

// Broadcast 255.255.255.255:<port>
// Sending net.DialUDP
// Receiving net.ListenUDP

func spawnBackup(cfg config.Config) error {
	port := cfg.Port
	id := cfg.ID

	dir, err := os.Getwd()
	if err != nil {
		return err
	}

	switch runtime.GOOS {
	case "darwin":
		cmdStr := fmt.Sprintf(`tell app "Terminal" to do script "cd %q; go run main.go -id=%q -port=%d"`, dir, id, port)
		return exec.Command("osascript", "-e", cmdStr).Run()
	case "linux":
		if _, err := exec.LookPath("gnome-terminal"); err == nil {
			cmd := exec.Command(
				"gnome-terminal",
				"--",
				"bash",
				"-c",
				fmt.Sprintf("cd %q && go run main.go -id=%q -port=%d; exec bash", dir, id, port),
			)
			return cmd.Start()
		}

		// Fallback
		if _, err := exec.LookPath("x-terminal-emulator"); err == nil {
			cmd := exec.Command(
				"x-terminal-emulator",
				"-e",
				fmt.Sprintf("bash -c 'cd %q && go run main.go -id=%q -port=%d; exec bash'", dir, id, port),
			)
			return cmd.Start()
		}

		return fmt.Errorf("No supperted terminal for linux")

	case "windows":
		return fmt.Errorf("windows not implemented")
	default:
		return fmt.Errorf("OS not supported")
	}
}

func RunProcessPairs(worldViewCh <-chan syncOrders.WorldView, restoreWorldViewCh chan syncOrders.WorldView, restart chan bool, cfg config.Config) {

	peerID_int, err := strconv.Atoi(cfg.ID)
	processPairPort := PROCESSPAIR_BASEPORT + peerID_int

	log.Printf("ProcessPairPort: %d", processPairPort)

	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("0.0.0.0:%d", processPairPort))
	if err != nil {
		log.Println("Failed to resolve UDP receive Addr")
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Println("Failed to connect UDP listener:", err)
		return
	}
	//defer conn.Close()

	state := State{IsPrimary: false}
	lastPrimary := time.Now()

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

	state.IsPrimary = true

	if state.CurrentWorldView.PeerID == "" {
		log.Println("Trying to restore empty id")
		state.CurrentWorldView.PeerID = cfg.ID
	}

	log.Printf("Sending restored worldview with cab orders: %+v", state.CurrentWorldView)
	restoreWorldViewCh <- state.CurrentWorldView

	if err := spawnBackup(cfg); err != nil {
		log.Println("Failed to spawn backup:", err)
	}

	sendAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("255.255.255.255:%d", processPairPort))
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

		case needRestart := <-restart:
			if needRestart {
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
			}
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
