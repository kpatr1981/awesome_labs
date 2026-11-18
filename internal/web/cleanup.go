package web

import (
	"context"
	"fmt"
	"io"
	"time"

	"awesome_labs/internal/lab"
)

// With delete_stopped_after_days set, lab VMs stopped for longer than that are
// deleted to free their disk: when the server starts, then once a day (and
// right away when the setting changes). An operation running at the time puts
// the round off until the next hourly look.

const cleanupEvery = 24 * time.Hour

func (s *server) reapStale(ctx context.Context) {
	var last time.Time
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		if _, days := s.live.get(); days > 0 && time.Since(last) >= cleanupEvery {
			if ran := s.deleteStale(ctx, days); ran {
				last = time.Now()
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-s.live.wait():
			last = time.Time{} // a new limit: look again now
		}
	}
}

// deleteStale deletes the stale stopped VMs as an operation. It reports
// whether this round is done: true when there was nothing to delete or the
// operation started, false when it has to wait for a busy one.
func (s *server) deleteStale(ctx context.Context, days int) bool {
	lctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	vms, err := s.Manager.VMs(lctx)
	if err != nil {
		return false
	}
	maxAge := time.Duration(days) * 24 * time.Hour
	if len(lab.StaleStopped(vms, maxAge, time.Now())) == 0 {
		return true
	}
	_, err = s.ops.start(s.ctx, "env", fmt.Sprintf("Deleting lab VMs stopped for more than %d days", days), "env", "",
		func(ctx context.Context, w io.Writer) (bool, error) {
			deleted, err := s.Manager.DeleteStaleStopped(ctx, maxAge, w)
			if err == nil && len(deleted) == 0 {
				fmt.Fprintln(w, "==> nothing to delete")
			}
			return false, err
		}, nil)
	return err == nil
}
