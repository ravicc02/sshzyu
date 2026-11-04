package service

import "time"

// Keep probing at most once a minute during a prolonged failure. Existing
// snapshots remain readable; no reset/delete is performed by the retry guard.
func schedulerFullRebuildRetryDelay(failures int) time.Duration {
	delay := time.Second
	for i := 1; i < failures && delay < time.Minute; i++ {
		delay *= 2
	}
	if delay > time.Minute {
		return time.Minute
	}
	return delay
}

// Empty queues need no one-second SQL transaction forever. New events are
// observed within five seconds with the default one-second base interval.
func schedulerIdlePollDelay(base time.Duration, emptyPolls uint32) time.Duration {
	if base >= 5*time.Second {
		return base
	}
	delay := base
	for i := uint32(0); i < emptyPolls && delay < 5*time.Second; i++ {
		delay *= 2
	}
	if delay > 5*time.Second {
		return 5 * time.Second
	}
	return delay
}

func (s *SchedulerSnapshotService) snapshotRebuildTimeout() time.Duration {
	if s.fullRebuildTimeout > 0 {
		return s.fullRebuildTimeout
	}
	return 30 * time.Second
}
