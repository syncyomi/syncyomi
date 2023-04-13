package server

import (
	"context"
	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/internal/logger"
	"github.com/SyncYomi/SyncYomi/internal/scheduler"
	"github.com/SyncYomi/SyncYomi/internal/update"
	"github.com/rs/zerolog"
	"sync"
	"time"
)

type Server struct {
	log    zerolog.Logger
	config *domain.Config

	scheduler     scheduler.Service
	updateService *update.Service

	stopWG sync.WaitGroup
	lock   sync.Mutex
}

func NewServer(log logger.Logger, config *domain.Config, scheduler scheduler.Service, updateSvc *update.Service) *Server {
	return &Server{
		log:           log.With().Str("module", "server").Logger(),
		config:        config,
		scheduler:     scheduler,
		updateService: updateSvc,
	}
}

func (s *Server) Start() error {
	go s.checkUpdates()

	// start cron scheduler
	s.scheduler.Start()

	return nil
}

func (s *Server) Shutdown() {
	s.log.Info().Msg("Shutting down server")

	// stop cron scheduler
	s.scheduler.Stop()
}

func (s *Server) checkUpdates() {
	if s.config.CheckForUpdates {
		time.Sleep(1 * time.Second)

		s.updateService.CheckUpdates(context.Background())
	}
}
