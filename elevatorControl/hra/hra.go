package hra

// - - - - - - Overview - - - - - - - - -

// Hall request assigner

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
)

// Struct members must be public in order to be accessible by json.Marshal/.Unmarshal
// This means they must start with a capital letter, so we need to use field renaming struct tags to make them camelCase

type HRAElevState struct {
	Behavior    string `json:"behaviour"`
	Floor       int    `json:"floor"`
	Direction   string `json:"direction"`
	CabRequests []bool `json:"cabRequests"`
}

type HRAInput struct {
	HallRequests [][2]bool               `json:"hallRequests"`
	States       map[string]HRAElevState `json:"states"`
}

type OrderAssignments map[string][][]bool

func Encode(input HRAInput) string {

	jsonBytes, err := json.Marshal(input)
	if err != nil {
		fmt.Println("json.Marshal error: ", err)
	}
	return string(jsonBytes)
}

func Decode(out string) OrderAssignments {

	var assignments OrderAssignments
	err := json.Unmarshal([]byte(out), &assignments)
	if err != nil {
		fmt.Println("json.Unmarshal error: ", err)
	}
	return assignments
}

func AssignOrders(jsonString string) string {

	hraExecutable := ""
	switch runtime.GOOS {
	case "linux":
		hraExecutable = "hall_request_assigner"
	case "windows":
		hraExecutable = "hall_request_assigner.exe"
	default:
		panic("OS not supported")
	}

	out, err := exec.Command("./"+hraExecutable, "-i", (jsonString)).CombinedOutput()
	if err != nil {
		fmt.Println("exec.Command error: ", err)
		fmt.Println(string(out))
	}
	return string(out)
}
