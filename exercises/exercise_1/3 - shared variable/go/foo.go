// Use `go run foo.go` to run your program

package main

import (
	. "fmt"
	"runtime"
)

var i = 0
var m = 0

func server(ch chan int, quit chan int) {
	for {
		select {
		case m := <-ch:
			i = i + m
		case <-quit:
			return
		}
	}
}

func incrementing(ch chan int, ch2 chan int, quit chan int) {
	//TODO: increment i 1000000 times
	for j := 0; j < 1000000; j++ {
		ch <- 1
	}
	<-ch2
	quit <- 1
}

func decrementing(ch chan int, ch2 chan int) {
	//TODO: decrement i 1000000 times
	for j := 0; j < 999997; j++ {
		ch <- -1
	}
	ch2 <- 1
}

func main() {
	// What does GOMAXPROCS do? What happens if you set it to 1?
	runtime.GOMAXPROCS(2)

	// Initialize a channel
	ch := make(chan int)
	ch2 := make(chan int)
	quit := make(chan int)

	// TODO: Spawn both functions as goroutines
	go incrementing(ch, ch2, quit)
	go decrementing(ch, ch2)
	server(ch, quit)

	// We have no direct way to wait for the completion of a goroutine (without additional synchronization of some sort)
	// We will do it properly with channels soon. For now: Sleep.
	//time.Sleep(500 * time.Millisecond)
	Println("The magic number is:", i)
}
