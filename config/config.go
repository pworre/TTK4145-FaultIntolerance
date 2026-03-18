package config

import (
	"flag"
	"fmt"
	"os"
)

// CONTENT: This file contains config for arguments in terminal when running the program.
// 			An ID must be set in order for an elevator to work on the network.
//			This must be unique and set manually at each PC.
//			The port and backup arguments have default options.

type Config struct {
	ID     string
	Port   int
	Backup bool
}

const STANDARD_PORT int = 15657

func ParseFlag() Config {
	id := flag.String("id", "-1", "Elevator ID")
	port := flag.Int("port", -1, "Simulator TCP port number")
	backup := flag.Bool("backup", false, "Start process as passive backup")

	flag.Parse()

	if *id == "-1" {
		fmt.Println("ERROR: You must provide --id")
		flag.Usage()
		os.Exit(1)
	}

	if *port == -1 {
		*port = STANDARD_PORT
	}

	return Config{
		ID:     *id,
		Port:   *port,
		Backup: *backup,
	}
}
