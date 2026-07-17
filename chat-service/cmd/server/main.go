package main

import (
	"log"

	"ws-ex/internal/app"
)

func main() {
	cfg := app.LoadConfig()
	if err := app.Run(cfg); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
