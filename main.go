package main

import (
	"fmt"
	"log"

	"github.com/travishegner/fdbsync/bird"
)

func main() {
	birdc, err := bird.NewSocketClient("/var/run/bird/bird.ctl", 4096)
	if err != nil {
		log.Fatalf("failed to create bird client: %v", err)
	}

	lines, err := birdc.Query("show route 10.1.0.0/24")
	if err != nil {
		log.Fatalf("failed to query bird client: %v", err)
	}

	for _, line := range lines {
		fmt.Println(line)
	}

	/*
			sig := make(chan os.Signal, 1)
			signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

		Control:
			for {
				select {
				case s := <-sig:
					fmt.Printf("received %v signal, quitting\n", s)
					break Control
				}
			}

			close(sig)
	*/
}
