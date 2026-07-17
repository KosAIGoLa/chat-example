package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ws-ex/controller"
	"ws-ex/database"
	"ws-ex/router"
	"ws-ex/service"

	"gorm.io/gorm"
)

// Run boots dependencies, serves HTTP, and shuts down on SIGINT/SIGTERM.
func Run(cfg Config) error {
	db := database.Init()

	authSvc := service.NewAuthService(db, cfg.JWTSecret)
	authCtrl := controller.NewAuthController(authSvc)

	hub := service.NewHub()
	natsSvc, err := service.NewNATSService(cfg.NATSURL, hub)
	if err != nil {
		return fmt.Errorf("nats: %w", err)
	}

	hub.SetNATS(natsSvc)
	authCtrl.SetHub(hub)

	offlineSvc := service.NewOfflineService(db, hub)
	hub.SetOffline(offlineSvc)
	natsSvc.SetOffline(offlineSvc)

	if cfg.MsgCryptoKey == "" {
		log.Printf("[Crypto] MSG_CRYPTO_KEY not set — deriving from JWT_SECRET")
	}
	msgCrypto := service.NewMsgCrypto(cfg.MsgCryptoKey)

	msgStore := service.NewMessageStore(db)
	friendSvc := service.NewFriendService(db, hub)
	friendSvc.SetOffline(offlineSvc)
	friendSvc.SetNATS(natsSvc)
	friendSvc.SetMessageStore(msgStore)

	groupSvc := service.NewGroupService(db, hub)

	chatSvc := service.NewChatService(hub, natsSvc, msgCrypto)
	chatSvc.SetMessageStore(msgStore)
	chatSvc.SetFriends(friendSvc)
	chatSvc.SetGroups(groupSvc)
	groupSvc.SetChatService(chatSvc)

	chatCtrl := controller.NewChatController(hub, chatSvc, natsSvc)
	chatCtrl.SetCrypto(msgCrypto)
	friendCtrl := controller.NewFriendController(friendSvc)
	groupCtrl := controller.NewGroupController(groupSvc)

	walletSvc := service.NewWalletService(db)
	rpSvc := service.NewRedPacketService(db, walletSvc, friendSvc, groupSvc, hub, natsSvc, msgStore)
	redPacketCtrl := controller.NewRedPacketController(rpSvc, walletSvc)

	lkSvc := service.NewLiveKitService()
	meetingSvc := service.NewMeetingService()
	livekitCtrl := controller.NewLiveKitController(lkSvc, hub, friendSvc, groupSvc, meetingSvc)
	livekitCtrl.SetChat(chatSvc)

	mediaSvc, err := service.NewMediaService(cfg.MediaDir)
	if err != nil {
		natsSvc.Close()
		return fmt.Errorf("media: %w", err)
	}
	mediaCtrl := controller.NewMediaController(mediaSvc)
	authCtrl.SetMedia(mediaSvc)
	groupCtrl.SetMedia(mediaSvc)

	engine := router.SetupRouter(
		chatCtrl, authCtrl, mediaCtrl, friendCtrl, groupCtrl, livekitCtrl, redPacketCtrl, authSvc,
	)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
		// Long-lived WebSockets — do not set WriteTimeout / IdleTimeout too aggressively.
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf(
			"Server starting on %s (NATS: %s, media: %s, shutdown-timeout: %s, msg-crypto: AES-GCM, livekit: %s)",
			cfg.Addr, cfg.NATSURL, cfg.MediaDir, cfg.ShutdownTimeout, lkSvc.URL(),
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err != nil {
			cleanup(hub, natsSvc, db)
			return fmt.Errorf("http: %w", err)
		}
		cleanup(hub, natsSvc, db)
		return nil
	case sig := <-sigCh:
		log.Printf("[Shutdown] signal %v — graceful stop (timeout %s)", sig, cfg.ShutdownTimeout)
	}

	// Stop accepting new HTTP/WS upgrades first.
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	// Close local WebSockets so clients reconnect (other instances / restart).
	// Done before http.Shutdown so upgrades stop and existing conns drain faster.
	hub.Close()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("[Shutdown] http force close: %v", err)
		_ = srv.Close()
	}

	cleanup(hub, natsSvc, db)
	log.Printf("[Shutdown] complete")
	return nil
}

func cleanup(hub *service.Hub, natsSvc *service.NATSService, db *gorm.DB) {
	// Idempotent: hub.Close may already have run.
	if hub != nil {
		hub.Close()
	}
	if natsSvc != nil {
		natsSvc.Close()
	}
	if db != nil {
		if sqlDB, err := db.DB(); err == nil && sqlDB != nil {
			_ = sqlDB.Close()
		}
	}
}
