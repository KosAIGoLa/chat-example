package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"ws-ex/dto"
	"ws-ex/service"
)

func AuthMiddleware(authSvc *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c)
		if token == "" {
			c.JSON(http.StatusUnauthorized, dto.APIResponseDTO{
				Code:    401,
				Message: "authorization header is required",
			})
			c.Abort()
			return
		}

		userID, username, err := authSvc.ValidateToken(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, dto.APIResponseDTO{
				Code:    401,
				Message: err.Error(),
			})
			c.Abort()
			return
		}

		c.Set("user_id", userID)
		c.Set("username", username)
		c.Next()
	}
}

// AuthOrQueryToken accepts Authorization: Bearer <token> or ?token=<jwt>.
// Useful for media URLs used as <audio src> which cannot set headers.
func AuthOrQueryToken(authSvc *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c)
		if token == "" {
			token = c.Query("token")
		}
		if token == "" {
			c.JSON(http.StatusUnauthorized, dto.APIResponseDTO{
				Code:    401,
				Message: "authorization required",
			})
			c.Abort()
			return
		}

		userID, username, err := authSvc.ValidateToken(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, dto.APIResponseDTO{
				Code:    401,
				Message: err.Error(),
			})
			c.Abort()
			return
		}

		c.Set("user_id", userID)
		c.Set("username", username)
		c.Next()
	}
}

func bearerToken(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return ""
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return ""
	}
	return parts[1]
}
