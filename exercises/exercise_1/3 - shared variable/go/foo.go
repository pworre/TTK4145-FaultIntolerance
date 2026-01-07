// Use `go run foo.go` to run your program

package main

import (
	. "fmt"
	"runtime"
	"time"
)

var i = 0

func server(ch chan int) {
    var m int = 0
    for {
        select{
            case m := <-ch:
                if m>0 {
                    i++
                }else if m<0 {
                    i--
                }
            case <-quit:
                return
        }
    }
}


func incrementing(ch chan int, ch2 chan int) {
	//TODO: increment i 1000000 times
	for j := 0; j < 1000000; j++ {
		ch <- 1
	}
    for {
        if <-ch2:
            break
    }
    quit <- 1
}

func decrementing(ch chan int) {
	//TODO: decrement i 1000000 times
	for j := 0; j < 1000000; j++ {
		ch <- -1
	}
    ch2 <- 1
}

func main() {
	// What does GOMAXPROCS do? What happens if you set it to 1?
	runtime.GOMAXPROCS(2)

    // Initialize a channel
    ch := make(chan int)
    quit := make(chan int)

	// TODO: Spawn both functions as goroutines
	go incrementing(ch)
	go decrementing(ch)

	// We have no direct way to wait for the completion of a goroutine (without additional synchronization of some sort)
	// We will do it properly with channels soon. For now: Sleep.
	//time.Sleep(500 * time.Millisecond)
	Println("The magic number is:", i)
}
