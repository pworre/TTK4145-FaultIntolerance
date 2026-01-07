# Sharing Variable
**What happens?**

We observe different results when running the program. This may be due to data race and the threads accessing the same memory. Hence, the thread for increasement will retrieve the value from the memory before the decreasement-thread is writing its value.

When using Go, we get the same results as C when allowing multiple threads. But setting a restriction to one thread, we get 0 since both functions is being runned on the same core. 