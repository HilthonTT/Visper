package jobs

import (
	"context"
	"sync"
	"time"

	"github.com/hilthontt/visper/services/api/application/usecases/file"
	"github.com/hilthontt/visper/services/api/application/usecases/room"
	"github.com/hilthontt/visper/services/api/infrastructure/logger"
	"go.uber.org/zap"
)

// RoomExpiryJob deletes rooms whose expiry has passed. Without it an expired
// room is only removed when someone happens to read it, so a room nobody opens
// again (and its members, messages and files) would stay in storage forever.
type RoomExpiryJob struct {
	roomUseCase room.RoomUseCase
	fileUseCase file.FileUseCase
	logger      *logger.Logger
	interval    time.Duration
	stopChan    chan struct{}
	stopOnce    sync.Once
}

func NewRoomExpiryJob(roomUseCase room.RoomUseCase, fileUseCase file.FileUseCase, logger *logger.Logger, interval time.Duration) *RoomExpiryJob {
	return &RoomExpiryJob{
		roomUseCase: roomUseCase,
		fileUseCase: fileUseCase,
		logger:      logger,
		interval:    interval,
		stopChan:    make(chan struct{}),
	}
}

func (j *RoomExpiryJob) Start(ctx context.Context) {
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()

	j.logger.Info("Room expiry job started", zap.Duration("interval", j.interval))

	j.run(ctx)
	for {
		select {
		case <-ticker.C:
			j.run(ctx)
		case <-j.stopChan:
			j.logger.Info("Room expiry job stopped")
			return
		case <-ctx.Done():
			return
		}
	}
}

func (j *RoomExpiryJob) Stop() {
	j.stopOnce.Do(func() { close(j.stopChan) })
}

func (j *RoomExpiryJob) run(ctx context.Context) {
	expired, err := j.roomUseCase.ExpireRooms(ctx)
	if err != nil {
		j.logger.Error("Room expiry job failed", zap.Error(err))
		return
	}
	if expired == 0 {
		return
	}

	j.logger.Info("Expired rooms deleted", zap.Int("count", expired))

	// The deleted rooms' files are now orphaned; remove them right away
	// rather than waiting for the next (6-hourly) file cleanup run.
	if err := j.fileUseCase.CleanupOrphanedFiles(ctx); err != nil {
		j.logger.Error("Cleanup of expired rooms' files failed", zap.Error(err))
	}
}
