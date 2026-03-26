package scheduler

import "context"

// Scheduler drives the 9am/9pm briefing pipeline.
// Full implementation in Week 4.
type Scheduler struct{}

func New() *Scheduler {
	return &Scheduler{}
}

// Start begins the cron loop. Blocks until ctx is cancelled.
func (s *Scheduler) Start(_ context.Context) error {
	return nil
}
