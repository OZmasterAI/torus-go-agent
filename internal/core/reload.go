package core

import (
	"context"
	"log"
	"os"
	"sync"
	"time"
)

// PromptReloader watches files and reloads the system prompt when they change.
type PromptReloader struct {
	agent    *Agent
	files    []string      // paths to watch
	interval time.Duration // poll interval
	mu       sync.RWMutex
	stopCh   chan struct{}
	builder  func() string // builds the prompt from sources
}

// NewPromptReloader creates a reloader that polls the given files.
// builder is called to reconstruct the full system prompt when any file changes.
func NewPromptReloader(agent *Agent, files []string, interval time.Duration, builder func() string) *PromptReloader {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &PromptReloader{
		agent:    agent,
		files:    files,
		interval: interval,
		stopCh:   make(chan struct{}),
		builder:  builder,
	}
}

// Start begins the poll loop in a goroutine. Call Stop() to end it.
func (r *PromptReloader) Start() {
	go r.poll()
}

// Stop signals the poll loop to exit.
func (r *PromptReloader) Stop() {
	close(r.stopCh)
}

// poll checks file modification times and rebuilds the prompt when changed.
func (r *PromptReloader) poll() {
	modTimes := r.snapshotModTimes()
	for {
		select {
		case <-r.stopCh:
			return
		case <-time.After(r.interval):
		}
		current := r.snapshotModTimes()
		changed := false
		for path, mt := range current {
			if prev, ok := modTimes[path]; !ok || !mt.Equal(prev) {
				changed = true
				break
			}
		}
		if !changed {
			continue
		}
		modTimes = current
		newPrompt := r.builder()
		r.agent.ReloadSystemPrompt(context.Background(), newPrompt)
		log.Printf("[reload] system prompt reloaded (%d bytes)", len(newPrompt))
	}
}

// snapshotModTimes returns the mod times of all watched files.
func (r *PromptReloader) snapshotModTimes() map[string]time.Time {
	m := make(map[string]time.Time, len(r.files))
	for _, f := range r.files {
		info, err := os.Stat(f)
		if err != nil {
			continue
		}
		m[f] = info.ModTime()
	}
	return m
}
