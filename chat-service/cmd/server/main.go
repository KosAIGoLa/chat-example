package main

import (
	"log"
	"os"

	"ws-ex/controller"
	"ws-ex/database"
	"ws-ex/router"
	"ws-ex/service"
)

func main() {
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://127.0.0.1:4222"
	}
	addr := os.Getenv("SERVER_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "change-me-in-production"
	}

	// Database
	db := database.Init()

	// Auth
	authSvc := service.NewAuthService(db, jwtSecret)
	authCtrl := controller.NewAuthController(authSvc)

	// Chat
	hub := service.NewHub()

	natsSvc, err := service.NewNATSService(natsURL, hub)
	if err != nil {
		log.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer natsSvc.Close()

	// Link NATS to Hub for KV presence management + heartbeat.
	hub.SetNATS(natsSvc)
	// Profile username changes update live presence via hub.
	authCtrl.SetHub(hub)

	// Message content encryption (AES-256-GCM). Prefer MSG_CRYPTO_KEY over JWT_SECRET.
	msgCrypto := service.NewMsgCrypto(os.Getenv("MSG_CRYPTO_KEY"))
	if os.Getenv("MSG_CRYPTO_KEY") == "" {
		log.Printf("[Crypto] MSG_CRYPTO_KEY not set — deriving from JWT_SECRET")
	}

	chatSvc := service.NewChatService(hub, natsSvc, msgCrypto)
	chatCtrl := controller.NewChatController(hub, chatSvc, natsSvc)
	chatCtrl.SetCrypto(msgCrypto)

	// Voice media storage
	mediaDir := os.Getenv("MEDIA_DIR")
	if mediaDir == "" {
		mediaDir = "./data/voice"
	}
	mediaSvc, err := service.NewMediaService(mediaDir)
	if err != nil {
		log.Fatalf("Failed to init media storage: %v", err)
	}
	mediaCtrl := controller.NewMediaController(mediaSvc)

	r := router.SetupRouter(chatCtrl, authCtrl, mediaCtrl, authSvc)

	log.Printf("Server starting on %s (NATS: %s, media: %s, msg-crypto: AES-GCM)", addr, natsURL, mediaDir)
	if err := r.Run(addr); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
