package middlewares

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	userUseCase "github.com/hilthontt/visper/services/api/application/usecases/user"
	"github.com/hilthontt/visper/services/api/domain/model"
	"github.com/hilthontt/visper/services/api/infrastructure/logger"
	"github.com/hilthontt/visper/services/api/infrastructure/security"
	"go.uber.org/zap"
)

const (
	UserContextKey = "user"
)

// UserMiddleware resolves the caller from their private user token (see
// security.UserIDFromToken). A request without one gets a new token in the
// session cookie; a token too short to be secret is rejected.
func UserMiddleware(userUC userUseCase.UserUseCase, logger *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := security.GetUserToken(c.Request)
		switch {
		case token == "":
			token = security.NewUserToken()
			security.SetUserToken(c.Writer, token)
		case !security.ValidUserToken(token):
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   "invalid_user_token",
				"message": fmt.Sprintf("The user token must be a random secret of %d to %d characters (not a user ID)", security.MinUserTokenLength, security.MaxUserTokenLength),
			})
			c.Abort()
			return
		}
		userID := security.UserIDFromToken(token)

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

func GetUserFromContext(c *gin.Context) (*model.User, bool) {
	user, exists := c.Get(UserContextKey)
	if !exists {
		return nil, false
	}

	u, ok := user.(*model.User)
	return u, ok
}
