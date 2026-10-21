// A separate opt-in harness: no desktop loop, window, or app lifecycle needed.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	mygomcp "github.com/auron-labs/mygo-mcp"
	"github.com/auron-labs/mygo-mcp/internal/demo"
	"github.com/egoist/mygo/ui"
)

func main() {
	address := flag.String("listen", "127.0.0.1:8080", "explicit loopback IP:port")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	fixture := mygomcp.Fixture{
		Name: "Example editor", Width: 321, Height: 201, Scale: 1.25,
		Factory: func() (func(*ui.Context), func(), error) {
			state := demo.New()
			return state.View, nil, nil
		},
	}
	err := mygomcp.Run(ctx, *address, []mygomcp.Fixture{fixture})
	stop()
	if err != nil {
		log.Printf("headless harness: %v", err)
		os.Exit(1)
	}
}
