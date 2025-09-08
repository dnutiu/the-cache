package main

import (
	"context"
	"fmt"
	"sync"
	"theCache/pkg"
	"time"
)

func main() {

	leCache := pkg.NewCache(10 * time.Minute)
	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func() {
			_, err := leCache.Fetch(context.Background(), "https://www.computerwoche.de/article/4049065/lernplattformen-fur-unternehmen-ein-kaufratgeber.html")
			if err == nil {
				fmt.Printf("Got the data: %d\n", i)
			} else {
				fmt.Printf("Got an error: %d\n", i)
			}
			wg.Done()
		}()
		wg.Wait()
	}
	fmt.Println(leCache.Stats())

}
