//go:build mygo_mcp

package main

import (
	"context"
	"log"
	"time"

	mygomcp "github.com/auron-labs/mygo-mcp"
)

func init() {
	startAutomation = func() func() {
		ctx, cancel := context.WithCancel(context.Background())
		server, err := mygomcp.Start(ctx, "127.0.0.1:7331")
		if err != nil {
			cancel()
			log.Fatal(err)
		}
		log.Printf("MCP: %s", server.URL())
		return func() {
			cancel()
			ctx, done := context.WithTimeout(context.Background(), 10*time.Second)
			defer done()
			if err := server.Close(ctx); err != nil {
				log.Print(err)
			}
		}
	}
}
