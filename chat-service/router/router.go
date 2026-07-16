package router

import (
	"github.com/gin-gonic/gin"

	"ws-ex/controller"
	"ws-ex/middleware"
	"ws-ex/service"
)

func SetupRouter(
	chatCtrl *controller.ChatController,
	authCtrl *controller.AuthController,
	mediaCtrl *controller.MediaController,
	authSvc *service.AuthService,
) *gin.Engine {
	r := gin.Default()

	// Auth routes (public)
	r.POST("/api/auth/register", authCtrl.Register)
	r.POST("/api/auth/login", authCtrl.Login)

	// Protected API group
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware(authSvc))
	{
		api.GET("/auth/me", authCtrl.GetMe)
		api.PUT("/auth/profile", authCtrl.UpdateProfile)

		// Message body encryption key (AES-GCM) for WebSocket chat content.
		api.GET("/crypto/key", chatCtrl.GetCryptoKey)

		api.POST("/groups/join", chatCtrl.JoinGroup)
		api.POST("/groups/leave", chatCtrl.LeaveGroup)
		api.GET("/groups/:group_id/members", chatCtrl.GetGroupMembers)
		api.GET("/users/online", chatCtrl.GetOnlineUsers)
		api.GET("/presence", chatCtrl.GetAllPresence)
		api.GET("/presence/:user_id", chatCtrl.GetPresence)
		api.GET("/history", chatCtrl.GetMessageHistory)

		// Voice messages
		api.POST("/voice", mediaCtrl.UploadVoice)
	}

	// Voice playback: Authorization header OR ?token= (for <audio src>).
	r.GET("/api/voice/:filename", middleware.AuthOrQueryToken(authSvc), mediaCtrl.GetVoice)

	// WebSocket (protected via query token)
	r.GET("/ws", func(c *gin.Context) {
		token := c.Query("token")
		if token == "" {
			c.JSON(400, gin.H{"code": 400, "message": "token is required"})
			c.Abort()
			return
		}
		userID, username, err := authSvc.ValidateToken(token)
		if err != nil {
			c.JSON(401, gin.H{"code": 401, "message": "invalid token"})
			c.Abort()
			return
		}
		c.Set("user_id", userID)
		c.Set("username", username)
		c.Next()
	}, chatCtrl.HandleWebSocket)

	return r
}
