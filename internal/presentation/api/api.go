package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/hilthontt/visper/internal/infrastructure/configs"
	"github.com/hilthontt/visper/internal/infrastructure/ratelimiter"
	healthHandler "github.com/hilthontt/visper/internal/presentation/handler/health"
	messagesHandler "github.com/hilthontt/visper/internal/presentation/handler/messages"
	roomHandler "github.com/hilthontt/visper/internal/presentation/handler/rooms"
	"go.uber.org/zap"
)

type Application struct {
	config          configs.Config
	roomHandler     roomHandler.Handler
	healthHandler   healthHandler.Handler
	messagesHandler messagesHandler.Handler
	logger          *zap.SugaredLogger
	ratelimiter     ratelimiter.Limiter
}

func NewApplication(
	config configs.Config,
	roomHandler roomHandler.Handler,
	healthHandler healthHandler.Handler,
	messagesHandler messagesHandler.Handler,
	logger *zap.SugaredLogger,
	ratelimiter ratelimiter.Limiter,
) *Application {
	return &Application{
		config:          config,
		roomHandler:     roomHandler,
		healthHandler:   healthHandler,
		messagesHandler: messagesHandler,
		logger:          logger,
		ratelimiter:     ratelimiter,
	}
}

func (app *Application) Mount() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	r.Use(app.rateLimiterMiddleware)
	r.Use(app.enableCors)

	r.Route("/api", func(r chi.Router) {
		r.Route("/rooms", func(r chi.Router) {
			r.Post("/", app.roomHandler.CreateRoomHandler)
			r.Get("/{roomId}", app.roomHandler.GetRoomHandler)
			r.Get("/{roomId}/join", app.roomHandler.JoinRoomHandler)
			r.Post("/{roomId}/boot", app.roomHandler.BootUserHandler)

			r.Post("/{roomId}/messages", app.messagesHandler.CreateNewMessageHandler)
			r.Delete("/{roomId}/messages/{messageId}", app.messagesHandler.DeleteMessageHandler)
		})

		r.Get("/health", app.healthHandler.GetHealth)
		r.Get("/healthz", app.healthHandler.GetHealth)
		r.Get("/ready", app.healthHandler.GetHealth)
		r.Get("/live", app.healthHandler.GetHealth)
	})

	return r
}

func (app *Application) Run(mux http.Handler) error {
	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", app.config.HTTP.Host, app.config.HTTP.Port),
		Handler:      mux,
		WriteTimeout: time.Second * 30,
		ReadTimeout:  time.Second * 10,
		IdleTimeout:  time.Minute,
	}

	shutdown := make(chan error)

	go func() {
		quit := make(chan os.Signal, 1)

		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		s := <-quit

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		app.logger.Infow("signal caught", "signal", s.String())

		shutdown <- srv.Shutdown(ctx)
	}()

	app.logger.Infow("server has started", "addr", srv.Addr)

	err := srv.ListenAndServe()
	if !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	err = <-shutdown
	if err != nil {
		return err
	}

	app.logger.Infow("server has stopped", "addr", srv.Addr)

	return nil
}
