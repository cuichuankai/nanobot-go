package cron

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
)

// Service manages scheduled jobs.
type Service struct {
	StorePath string
	OnJob     func(CronJob)
	store     *CronStore
	running   bool
	stopChan  chan struct{}
	mu        sync.RWMutex
}

// NewService creates a new cron service.
func NewService(storePath string, onJob func(CronJob)) *Service {
	return &Service{
		StorePath: storePath,
		OnJob:     onJob,
		stopChan:  make(chan struct{}),
	}
}

func (s *Service) nowMs() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

func (s *Service) computeNextRun(schedule CronSchedule, nowMs int64) int64 {
	if schedule.Kind == "at" {
		return schedule.AtMs
	}

	if schedule.Kind == "every" {
		if schedule.EveryMs <= 0 {
			return 0
		}
		return nowMs + schedule.EveryMs
	}

	if schedule.Kind == "cron" && schedule.Expr != "" {
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		sched, err := parser.Parse(schedule.Expr)
		if err != nil {
			log.Printf("Error parsing cron expr '%s': %v", schedule.Expr, err)
			return 0
		}
		now := time.Unix(0, nowMs*int64(time.Millisecond))
		next := sched.Next(now)
		return next.UnixNano() / int64(time.Millisecond)
	}

	return 0
}

func (s *Service) loadStore() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.store != nil {
		return
	}

	s.store = &CronStore{Version: 1, Jobs: []CronJob{}}

	if _, err := os.Stat(s.StorePath); os.IsNotExist(err) {
		return
	}

	data, err := ioutil.ReadFile(s.StorePath)
	if err != nil {
		log.Printf("Failed to load cron store: %v", err)
		return
	}

	if err := json.Unmarshal(data, s.store); err != nil {
		log.Printf("Failed to parse cron store: %v", err)
	}
}

func (s *Service) saveStore() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.store == nil {
		return
	}

	dir := filepath.Dir(s.StorePath)
	os.MkdirAll(dir, 0755)

	data, err := json.MarshalIndent(s.store, "", "  ")
	if err != nil {
		log.Printf("Failed to marshal cron store: %v", err)
		return
	}

	if err := ioutil.WriteFile(s.StorePath, data, 0644); err != nil {
		log.Printf("Failed to save cron store: %v", err)
	}
}

// Start starts the cron service.
func (s *Service) Start() {
	s.loadStore()
	s.recomputeNextRuns()
	s.saveStore()
	s.running = true
	go s.loop()
	log.Printf("Cron service started with %d jobs", len(s.store.Jobs))
}

// Stop stops the cron service.
func (s *Service) Stop() {
	s.running = false
	close(s.stopChan)
}

func (s *Service) recomputeNextRuns() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.store == nil {
		return
	}
	now := s.nowMs()
	for i := range s.store.Jobs {
		job := &s.store.Jobs[i]
		if job.Enabled {
			job.State.NextRunAtMs = s.computeNextRun(job.Schedule, now)
		}
	}
}

func (s *Service) getNextWakeMs() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.store == nil {
		return 0
	}

	var minNext int64 = 0
	for _, job := range s.store.Jobs {
		if job.Enabled && job.State.NextRunAtMs > 0 {
			if minNext == 0 || job.State.NextRunAtMs < minNext {
				minNext = job.State.NextRunAtMs
			}
		}
	}
	return minNext
}

func (s *Service) loop() {
	for {
		if !s.running {
			return
		}

		nextWake := s.getNextWakeMs()
		now := s.nowMs()
		
		var delay time.Duration
		if nextWake > 0 {
			if nextWake > now {
				delay = time.Duration(nextWake-now) * time.Millisecond
			} else {
				delay = 0
			}
		} else {
			// No jobs scheduled, check periodically
			delay = 10 * time.Second 
		}

		// Cap max delay to avoid sleeping too long if new jobs added
		if delay > 10*time.Second {
			delay = 10 * time.Second
		}

		select {
		case <-s.stopChan:
			return
		case <-time.After(delay):
			s.processJobs()
		}
	}
}

func (s *Service) processJobs() {
	s.mu.Lock()
	// Copy jobs to avoid holding lock during execution
	// But we need to update state, so we just identify indices
	var dueIndices []int
	now := s.nowMs()

	if s.store == nil {
		s.mu.Unlock()
		return
	}

	for i, job := range s.store.Jobs {
		if job.Enabled && job.State.NextRunAtMs > 0 && now >= job.State.NextRunAtMs {
			dueIndices = append(dueIndices, i)
		}
	}
	s.mu.Unlock()

	for _, idx := range dueIndices {
		// Re-acquire lock to get fresh job data and update state
		s.mu.Lock()
		// Check bounds and state again just in case
		if idx >= len(s.store.Jobs) {
			s.mu.Unlock()
			continue
		}
		job := s.store.Jobs[idx]
		s.mu.Unlock()

		s.executeJob(&job)

		// Update state after execution
		s.mu.Lock()
		// Need to find job again by ID because slice might have changed (though we are single threaded in loop)
		// But Add/Remove might happen concurrently.
		// Safe approach: find by ID
		storeIdx := -1
		for i, j := range s.store.Jobs {
			if j.ID == job.ID {
				storeIdx = i
				break
			}
		}

		if storeIdx != -1 {
			s.store.Jobs[storeIdx] = job // Update job in store with new state
			
			// Handle one-shot
			if job.Schedule.Kind == "at" {
				if job.DeleteAfterRun {
					// Remove
					s.store.Jobs = append(s.store.Jobs[:storeIdx], s.store.Jobs[storeIdx+1:]...)
				} else {
					s.store.Jobs[storeIdx].Enabled = false
					s.store.Jobs[storeIdx].State.NextRunAtMs = 0
				}
			} else {
				// Recompute next
				s.store.Jobs[storeIdx].State.NextRunAtMs = s.computeNextRun(job.Schedule, s.nowMs())
			}
		}
		s.mu.Unlock()
	}

	if len(dueIndices) > 0 {
		s.saveStore()
	}
}

func (s *Service) executeJob(job *CronJob) {
	log.Printf("Cron: executing job '%s' (%s)", job.Name, job.ID)
	startMs := s.nowMs()

	defer func() {
		if r := recover(); r != nil {
			log.Printf("Cron: panic executing job: %v", r)
			job.State.LastStatus = "error"
			job.State.LastError = fmt.Sprintf("panic: %v", r)
		}
	}()

	if s.OnJob != nil {
		s.OnJob(*job)
	}

	job.State.LastStatus = "ok"
	job.State.LastError = ""
	job.State.LastRunAtMs = startMs
	job.UpdatedAtMs = s.nowMs()
}

// Public API

func (s *Service) ListJobs() []CronJob {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.store == nil {
		return nil
	}
	
	// Return copy
	jobs := make([]CronJob, len(s.store.Jobs))
	copy(jobs, s.store.Jobs)
	
	// Sort
	sort.Slice(jobs, func(i, j int) bool {
		n1 := jobs[i].State.NextRunAtMs
		n2 := jobs[j].State.NextRunAtMs
		if n1 == 0 { return false }
		if n2 == 0 { return true }
		return n1 < n2
	})
	
	return jobs
}

func (s *Service) AddJob(name string, schedule CronSchedule, message string, deliver bool, channel, to string, deleteAfterRun bool) CronJob {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.store == nil {
		s.store = &CronStore{Version: 1, Jobs: []CronJob{}}
	}

	now := s.nowMs()
	id := uuid.New().String()[:8]

	job := CronJob{
		ID:      id,
		Name:    name,
		Enabled: true,
		Schedule: schedule,
		Payload: CronPayload{
			Kind:    "agent_turn",
			Message: message,
			Deliver: deliver,
			Channel: channel,
			To:      to,
		},
		State: CronJobState{
			NextRunAtMs: s.computeNextRun(schedule, now),
		},
		CreatedAtMs:    now,
		UpdatedAtMs:    now,
		DeleteAfterRun: deleteAfterRun,
	}

	s.store.Jobs = append(s.store.Jobs, job)
	
	// Trigger save implicitly by returning, caller might assume it persists? 
	// Ideally we save here.
	// We can't call s.saveStore() because we hold the lock.
	// Refactor saveStore to internal _saveStore
	s.saveStoreLocked()
	
	return job
}

func (s *Service) saveStoreLocked() {
	if s.store == nil {
		return
	}
	dir := filepath.Dir(s.StorePath)
	os.MkdirAll(dir, 0755)
	data, _ := json.MarshalIndent(s.store, "", "  ")
	ioutil.WriteFile(s.StorePath, data, 0644)
}

func (s *Service) RemoveJob(jobID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.store == nil {
		return false
	}

	newJobs := make([]CronJob, 0, len(s.store.Jobs))
	found := false
	for _, job := range s.store.Jobs {
		if job.ID == jobID {
			found = true
			continue
		}
		newJobs = append(newJobs, job)
	}

	if found {
		s.store.Jobs = newJobs
		s.saveStoreLocked()
	}
	return found
}
