package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/msmecodex/quicframe/framework"
	"github.com/msmecodex/quicframe/middleware"
)

func main() {
	// 1. Setup Application
	app := quicframe.New()

	// 2. Add MTLS authentication middleware
	// This will extract the NodeID from the common name of the peer certificate.
	app.Use(middleware.MTLSAuth())

	// 3. Logger middleware (demonstrates fixed context propagation)
	app.Use(middleware.Logger())

	// 4. Define a secure handler
	app.GET("/secure", func(ctx *quicframe.Context) error {
		nodeID, _ := ctx.Get("node_id")
		return ctx.Send(200, []byte(fmt.Sprintf("Hello, %s! Access granted via mTLS.", nodeID)))
	})

	// 5. Start server with automated PKI
	// This will manage certificate generation, disk persistence, and 14-day rotation.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	fmt.Println("Starting QuicFrame server with automated PKI on :4433...")
	
	// We use a directory for certificate persistence
	certDir := "./.certs"
	if err := os.MkdirAll(certDir, 0700); err != nil {
		log.Fatal(err)
	}

	// ListenWithPKI encapsulates pki.Manager logic
	go func() {
		if err := app.ListenWithPKI(ctx, ":4433", "", "satellite-01", "localhost"); err != nil {
			log.Printf("Server error: %v", err)
		}
	}()

	// 6. Demonstrate a client dialing with automated mTLS (Simulation)
	time.Sleep(2 * time.Second)
	fmt.Println("\n--- Client Simulation ---")
	
	// In a real scenario, the client would use its own PKI manager to load its identity.
	// For this demo, we'll just show the Dial logic.
	
	// client, err := quicframe.Dial(ctx, "localhost:4433", clientTlsCfg, nil)
	// if err != nil { ... }
	
	<-ctx.Done()
	fmt.Println("Shutting down...")
}
