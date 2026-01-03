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

func UserMiddleware(userUC userUseCase.UserUseCase, logger *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := getUserIDFromRequest(c)
		if userID == "" {
			userID = uuid.NewString()
			setUserIDCookie(c, userID)
			logger.Debug("generated new user ID", zap.String("userID", userID))
		}

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

		c.Set(UserContextKey, user)

		c.Next()
	}
}

func getUserIDFromRequest(c *gin.Context) string {
	if headerUserID := c.GetHeader("X-User-ID"); headerUserID != "" {
		return headerUserID
	}

	return security.GetUserID(c.Request)
}

func setUserIDCookie(c *gin.Context, userID string) {
	security.SetUserID(c.Writer, userID)
}

func GetUserFromContext(c *gin.Context) (*model.User, bool) {
	user, exists := c.Get(UserContextKey)
	if !exists {
		return nil, false
	}

	u, ok := user.(*model.User)
	return u, ok
}
