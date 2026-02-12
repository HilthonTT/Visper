package profiler

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"
)

type AdaptiveProfiler struct {
	// Configuration
	profileDir      string
	cpuThreshold    float64 // CPU threshold to trigger profiling (0-1)
	memThreshold    float64 // Memory threshold (0-1)
	minInterval     time.Duration
	profileDuration time.Duration

	// State
	lastProfile time.Time
	mutex       sync.Mutex
	isRunning   bool

	// CPU tracking
	lastCPUTime  time.Time
	lastCPUUsage float64
}

func NewAdaptiveProfiler(profileDir string) *AdaptiveProfiler {
	return &AdaptiveProfiler{
		profileDir:      profileDir,
		cpuThreshold:    0.70, // Start profiling at 70% CPU
		memThreshold:    0.80, // Start profiling at 80% memory
		minInterval:     10 * time.Minute,
		profileDuration: 30 * time.Second,
		lastProfile:     time.Time{},
		lastCPUTime:     time.Now(),
	}
}

func (p *AdaptiveProfiler) Start(ctx context.Context) {
	go p.monitor(ctx)
}

func (p *AdaptiveProfiler) monitor(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.checkAndProfile()
		case <-ctx.Done():
			return
		}
	}
}

func (p *AdaptiveProfiler) checkAndProfile() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.isRunning {
		return
	}

	// Check if we're within minimum interval
	if time.Since(p.lastProfile) < p.minInterval {
		return
	}

	// Check memory usage
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	memUsage := float64(m.Alloc) / float64(m.Sys)

	// Check CPU usage
	cpuUsage := p.getCPUUsage()

	// If thresholds are exceeded, profile
	if cpuUsage > p.cpuThreshold || memUsage > p.memThreshold {
		fmt.Printf("Thresholds exceeded - CPU: %.2f%%, Mem: %.2f%% - Starting profiling\n",
			cpuUsage*100, memUsage*100)
		p.isRunning = true
		go p.captureProfiles()
	}
}

func (p *AdaptiveProfiler) getCPUUsage() float64 {
	// Sample goroutine count as a proxy for CPU activity
	// This is a simplified approach that works across platforms
	numGoroutines := float64(runtime.NumGoroutine())
	numCPU := float64(runtime.NumCPU())

	// Calculate a normalized usage based on goroutines per CPU
	// This isn't perfect but gives us a relative measure
	usage := numGoroutines / (numCPU * 10) // Assume 10 goroutines per CPU is "normal"

	// Cap at 1.0
	if usage > 1.0 {
		usage = 1.0
	}

	// Smooth the value with exponential moving average
	now := time.Now()
	timeDelta := now.Sub(p.lastCPUTime).Seconds()
	if timeDelta > 0 {
		alpha := 0.3 // Smoothing factor
		p.lastCPUUsage = alpha*usage + (1-alpha)*p.lastCPUUsage
		p.lastCPUTime = now
	}

	return p.lastCPUUsage
}

func (p *AdaptiveProfiler) captureProfiles() {
	timestamp := time.Now().Format("20060102-150405")

	// Ensure profile directory exists
	if err := os.MkdirAll(p.profileDir, 0755); err != nil {
		fmt.Printf("Error creating profile directory: %v\n", err)
		p.mutex.Lock()
		p.isRunning = false
		p.mutex.Unlock()
		return
	}

	// Capture CPU profile
	cpuFile, err := os.Create(fmt.Sprintf("%s/cpu-%s.pprof", p.profileDir, timestamp))
	if err != nil {
		fmt.Printf("Error creating CPU profile: %v\n", err)
	} else {
		runtime.GC() // Run GC before profiling
		if err := pprof.StartCPUProfile(cpuFile); err != nil {
			fmt.Printf("Error starting CPU profile: %v\n", err)
		} else {
			time.Sleep(p.profileDuration) // Profile for N seconds
			pprof.StopCPUProfile()
		}
		cpuFile.Close()
		fmt.Printf("CPU profile saved: cpu-%s.pprof\n", timestamp)
	}

	// Capture memory profile
	memFile, err := os.Create(fmt.Sprintf("%s/mem-%s.pprof", p.profileDir, timestamp))
	if err != nil {
		fmt.Printf("Error creating memory profile: %v\n", err)
	} else {
		runtime.GC() // Run GC before profiling
		if err := pprof.WriteHeapProfile(memFile); err != nil {
			fmt.Printf("Error writing memory profile: %v\n", err)
		}
		memFile.Close()
		fmt.Printf("Memory profile saved: mem-%s.pprof\n", timestamp)
	}

	// Capture goroutine profile
	goroutineFile, err := os.Create(fmt.Sprintf("%s/goroutine-%s.pprof", p.profileDir, timestamp))
	if err != nil {
		fmt.Printf("Error creating goroutine profile: %v\n", err)
	} else {
		profile := pprof.Lookup("goroutine")
		if profile != nil {
			if err := profile.WriteTo(goroutineFile, 0); err != nil {
				fmt.Printf("Error writing goroutine profile: %v\n", err)
			}
		}
		goroutineFile.Close()
		fmt.Printf("Goroutine profile saved: goroutine-%s.pprof\n", timestamp)
	}

	// Mark profiling complete
	p.mutex.Lock()
	p.lastProfile = time.Now()
	p.isRunning = false
	p.mutex.Unlock()
}
