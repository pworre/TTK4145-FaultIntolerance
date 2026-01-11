
package main

import "fmt"
import "time"
import "sync"


func producer(ch chan<- int){

    for i := 0; i < 10; i++ {
        time.Sleep(100 * time.Millisecond)
        fmt.Printf("[producer]: pushing %d\n", i)
        ch <- i
    }

}

func consumer(ch <-chan int){

    time.Sleep(1 * time.Second)
    for {
        i := <- ch
        fmt.Printf("[consumer]: %d\n", i)
        time.Sleep(50 * time.Millisecond)
    }
    
}


func main(){
    ch := make(chan int, 5)

    wg := sync.WaitGroup{}
    wg.Add(2)
    
    go func(){
        defer wg.Done()
        consumer(ch)
    }()
    go func(){
        defer wg.Done()
        producer(ch)
    }()
    

    wg.Wait()
}