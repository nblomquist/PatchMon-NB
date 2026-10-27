package commands

import (
	"context"
	"time"
)

// finishPatchReport retries terminal delivery within the caller's deadline.
// A successful package operation must still refresh inventory when delivery
// fails. Keep that refresh independent of the delivery context: its deadline
// may already have expired during a proxy outage.
func finishPatchReport(ctx context.Context, send func(context.Context) error, refresh func(), retryInterval time.Duration) error {
	if refresh != nil {
		defer refresh()
	}
	for {
		err := send(ctx)
		if err == nil {
			return nil
		}
		logger.WithError(err).Warn("Failed to deliver final patch report; retrying until deadline")
		timer := time.NewTimer(retryInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return err
		case <-timer.C:
			if ctx.Err() != nil {
				return err
			}
		}
	}
}

func refreshPostPatchInventory() {
	logger.Info("Sending post-patch report to refresh package lists...")
	reportDone := make(chan error, 1)
	go func() { reportDone <- sendReport(false) }()
	select {
	case err := <-reportDone:
		if err != nil {
			logger.WithError(err).Warn("Post-patch report failed")
		} else {
			logger.Info("Post-patch report sent successfully")
		}
	case <-time.After(2 * time.Minute):
		logger.Warn("Post-patch report timed out after 2 minutes; will retry on next scheduled report")
	}
}
