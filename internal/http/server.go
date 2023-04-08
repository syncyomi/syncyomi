package http

import (
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/sessions"
	"github.com/kaiserbh/tachiyomi-sync-server/internal/config"
	"github.com/kaiserbh/tachiyomi-sync-server/internal/database"
	"github.com/kaiserbh/tachiyomi-sync-server/internal/logger"
	"github.com/kaiserbh/tachiyomi-sync-server/web"
	"github.com/r3labs/sse/v2"
	"github.com/rs/cors"
	"github.com/rs/zerolog"
	"net"
	"net/http"
)

type Server struct {
	log zerolog.Logger
	sse *sse.Server
	db  *database.DB

	config      *config.AppConfig
	cookieStore *sessions.CookieStore

	version string
	commit  string
	date    string

	apiService          apikeyService
	authService         authService
	notificationService notificationService
	updateService       updateService

	deviceService deviceService
}

func NewServer(log logger.Logger, config *config.AppConfig, sse *sse.Server, db *database.DB, version string, commit string, date string,
	apiService apikeyService, authService authService, notificationSvc notificationService, updateSvc updateService, deviceService deviceService) Server {
	return Server{
		log:     log.With().Str("module", "http").Logger(),
		config:  config,
		sse:     sse,
		db:      db,
		version: version,
		commit:  commit,
		date:    date,

		cookieStore: sessions.NewCookieStore([]byte(config.Config.SessionSecret)),

		apiService:          apiService,
		authService:         authService,
		notificationService: notificationSvc,
		updateService:       updateSvc,
		deviceService:       deviceService,
	}
}

func (s Server) Open() error {
	addr := fmt.Sprintf("%v:%v", s.config.Config.Host, s.config.Config.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	server := http.Server{
		Handler: s.Handler(),
	}

	s.log.Info().Msgf("Starting server. Listening on %s", listener.Addr().String())

	return server.Serve(listener)
}

func (s Server) Handler() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(LoggerMiddleware(&s.log))

	c := cors.New(cors.Options{
		AllowCredentials:   true,
		AllowedMethods:     []string{"HEAD", "OPTIONS", "GET", "POST", "PUT", "PATCH", "DELETE"},
		AllowOriginFunc:    func(origin string) bool { return true },
		OptionsPassthrough: true,
		// Enable Debugging for testing, consider disabling in production
		Debug: false,
	})

	r.Use(c.Handler)

	encoder := encoder{}

	web.RegisterHandler(r)

	r.Route("/api", func(r chi.Router) {
		r.Route("/auth", newAuthHandler(encoder, s.log, s.config.Config, s.cookieStore, s.authService).Routes)
		r.Route("/healthz", newHealthHandler(encoder, s.db).Routes)

		r.Group(func(r chi.Router) {
			r.Use(s.IsAuthenticated)

			r.Route("/config", newConfigHandler(encoder, s, s.config).Routes)
			r.Route("/keys", newAPIKeyHandler(encoder, s.apiService).Routes)
			r.Route("/logs", newLogsHandler(s.config).Routes)
			r.Route("/notification", newNotificationHandler(encoder, s.notificationService).Routes)
			r.Route("/updates", newUpdateHandler(encoder, s.updateService).Routes)
			r.Route("/device", newDeviceHandler(encoder, s.deviceService).Routes)

			r.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {

				// inject CORS headers to bypass checks
				s.sse.Headers = map[string]string{
					"Content-Type":      "text/event-stream",
					"Cache-Control":     "no-cache",
					"Connection":        "keep-alive",
					"X-Accel-Buffering": "no",
				}

				s.sse.ServeHTTP(w, r)
			})
		})

		// serve the parsed index.html
		r.Get("/", s.index)
		r.Get("/*", s.index)
	})

	return r
}

func (s Server) index(w http.ResponseWriter, r *http.Request) {
	p := web.IndexParams{
		Title:   "Dashboard",
		Version: s.version,
		BaseUrl: s.config.Config.BaseURL,
	}
	web.Index(w, p)
}
