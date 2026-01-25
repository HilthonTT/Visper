package jobs

import (
	"context"
	"time"

	"github.com/hilthontt/visper/api/application/usecases/file"
	"github.com/hilthontt/visper/api/infrastructure/logger"
	"go.uber.org/zap"
)

type FileCleanupJob struct {
	fileUseCase file.FileUseCase
	logger      *logger.Logger
	interval    time.Duration
	stopChan    chan struct{}
}

func NewFileCleanupJob(fileUseCase file.FileUseCase, logger *logger.Logger, interval time.Duration) *FileCleanupJob {
	return &FileCleanupJob{
		fileUseCase: fileUseCase,
		logger:      logger,
		interval:    interval,
		stopChan:    make(chan struct{}),
	}
}

func (j *FileCleanupJob) Start(ctx context.Context) {
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()

	j.logger.Info("File cleanup job started",
		zap.Duration("interval", j.interval),
	)

	j.runCleanup(ctx)

	for {
		select {
		case <-ticker.C:
			j.runCleanup(ctx)
		case <-j.stopChan:
			j.logger.Info("File cleanup job stopped")
			return
		case <-ctx.Done():
			j.logger.Info("File cleanup job context cancelled")
			return
		}
	}
}

func (j *FileCleanupJob) Stop() {
	close(j.stopChan)
}

func (j *FileCleanupJob) runCleanup(ctx context.Context) {
	j.logger.Info("Running file cleanup job")

	startTime := time.Now()

	if err := j.fileUseCase.CleanupOrphanedFiles(ctx); err != nil {
		j.logger.Error("File cleanup job failed",
			zap.Error(err),
			zap.Duration("duration", time.Since(startTime)),
		)
		return
	}

	j.logger.Info("File cleanup job completed successfully",
		zap.Duration("duration", time.Since(startTime)),
	)
}
