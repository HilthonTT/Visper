package middlewares

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	userUseCase "github.com/hilthontt/visper/api/application/usecases/user"
	"github.com/hilthontt/visper/api/domain/model"
	"github.com/hilthontt/visper/api/infrastructure/logger"
	"github.com/hilthontt/visper/api/infrastructure/security"
	"go.uber.org/zap"
)

const (
	UserContextKey = "user"
)

// UserMiddleware ensures every request has a valid user, creating one if necessary
func UserMiddleware(userUC userUseCase.UserUseCase, logger *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get or create user ID from cookie/header
		userID := getUserIDFromRequest(c)
		if userID == "" {
			userID = uuid.NewString()
			setUserIDCookie(c, userID)
			logger.Debug("generated new user ID", zap.String("userID", userID))
		}

		// Get or create user in Redis
		user, err := userUC.GetOrCreateUser(c.Request.Context(), userID)
		if err != nil {
			logger.Error("failed to get or create user", zap.Error(err), zap.String("userID", userID))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "internal_server_error",
				"message": "Failed to initialize user session",
			})
			c.Abort()
			return
		}

		// Set user in context for use in controllers
		c.Set(UserContextKey, user)

		c.Next()
	}
}

// getUserIDFromRequest extracts user ID from header or cookie
func getUserIDFromRequest(c *gin.Context) string {
	// Check header first (for API/WebSocket clients)
	if headerUserID := c.GetHeader("X-User-ID"); headerUserID != "" {
		return headerUserID
	}

	// Fall back to cookie
	return security.GetUserID(c.Request)
}

// setUserIDCookie sets the user ID cookie
func setUserIDCookie(c *gin.Context, userID string) {
	security.SetUserID(c.Writer, userID)
}

// GetUserFromContext extracts the user from the Gin context
func GetUserFromContext(c *gin.Context) (*model.User, bool) {
	user, exists := c.Get(UserContextKey)
	if !exists {
		return nil, false
	}

	u, ok := user.(*model.User)
	return u, ok
}
