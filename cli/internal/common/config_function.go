package common

import (
	"log/slog"
	"os"
	"runtime"

	"github.com/hilthontt/visper/cli/config"
	"github.com/hilthontt/visper/cli/internal/utils"
)

func InitialConfig() {
	file, err := os.OpenFile(config.LogFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)

	// TODO : This could be improved if we want to make superfile more resilient to errors
	// For example if the log file directories have access issues.
	// we could pass a dummy object to log.SetOutput() and the app would still function.
	if err != nil {
		utils.PrintfAndExitf("Error while opening superfile.log file : %v", err)
	}

	LoadConfigFile()

	logLevel := slog.LevelInfo
	if Config.Debug {
		logLevel = slog.LevelDebug
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(
		file, &slog.HandlerOptions{Level: logLevel})))

	LoadHotkeysFile(Config.IgnoreMissingFields)

	LoadInitialPrerenderedVariables()

	cwd, err := os.Getwd()
	if err != nil {
		slog.Error("cannot get current working directory", "error", err)
		cwd = config.HomeDir
	}

	slog.Debug("Directory configuration", "cwd", cwd)
	printRuntimeInfo()
}

func printRuntimeInfo() {
	slog.Debug("Runtime information", "runtime.GOOS", runtime.GOOS)
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	slog.Debug("Memory usage",
		"alloc_bytes", memStats.Alloc,
		"total_alloc_bytes", memStats.TotalAlloc,
		"heap_objects", memStats.HeapObjects,
		"sys_bytes", memStats.Sys)
}
