Exercise 1 - Theory questions
-----------------------------

### Concepts

What is the difference between *concurrency* and *parallelism*?
> *Concurrency deals with multiple tasks at once, but on a single core, and switching between the tasks rapidly creating an illusion of parallelism. Parallelism means executing multiple tasks on multiple cores.*

What is the difference between a *race condition* and a *data race*? 
> *A race condition occurs when data of a memory adress is dependent on the timing it is accessed, for example if two threads use the same data but the first thread requires updated data before the second thread has updated the data. This may result in unexpected behaviour of the program.* 

>*Data race occurs when two ore more threads try to access the same memory at the same time, and at least one of the instructions is a write-instruction.*
 
*Very* roughly - what does a *scheduler* do, and how does it do it?
> *A scheduler sets up the order of when different tasks should be executed.* 


### Engineering

Why would we use multiple threads? What kinds of problems do threads solve?
> *If we have enough tasks that can be done independently of another, then we can save a lot of time by executing many of them at the same time, on different threads in parallell.*

Some languages support "fibers" (sometimes called "green threads") or "coroutines"? What are they, and why would we rather use them over threads?
> *Fibers extend concurrency to threads in a processor. Fibers, unlike threads, are able to cooperate during execution, such that they can switch between them without involving the scheduler. Fibers are OS-managed, while coroutines are fibers that are not OS-managed. These routines make sense where we have a lot of parallell tasks, but some of the tasks depend on other ones.*

Does creating concurrent programs make the programmer's life easier? Harder? Maybe both?
> *Harder, due to the constant switiching between threads. The programmer needs to account for race conditions and data races.*

What do you think is best - *shared variables* or *message passing*?
> *Message passing seems to make it easier to spot and prevent race conditions, but is perhaps more difficult to get to run reliably, corrctly and quickly. Shared variables seems easier to make correct and fast programs, but demand more when fishing out race conditions. Based on this, we think message passing is best.*


