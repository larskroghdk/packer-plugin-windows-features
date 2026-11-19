package provisioner

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/larskroghdk/packer-plugin-windows-features/internal/facts"
)

const (
	restartCommand      = `shutdown.exe /r /f /t 5 /c "packer-plugin-windows-features: restart to complete feature changes"`
	maxRestarts         = 3
	probeAttemptTimeout = 2 * time.Minute
)

// timing holds the waits of the restart loop, so tests can shorten them.
type timing struct {
	probeInterval time.Duration
	settle        time.Duration
}

var defaultTiming = timing{probeInterval: 10 * time.Second, settle: 30 * time.Second}

// restart restarts the guest the way Packer's windows-restart provisioner
// does: it issues a delayed shutdown.exe restart, waits until a new WinRM
// command reports a new boot time that stays the same across a settle
// period, so a second restart Windows makes on its own while configuring
// features is waited for too, and then uploads the guest scripts again.
//
// Returns:
//   - error: The boot time cannot be read, the restart command failed, the
//     guest did not come back within restart_timeout, or the scripts cannot
//     be uploaded again.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *run) restart(ctx context.Context) error {
	before, err := r.probe(ctx)
	if err != nil {
		return fmt.Errorf("cannot read the boot time before restarting: %w", err)
	}
	r.ui.Say("Restarting the machine...")
	if err := r.remote.exec(ctx, restartCommand); err != nil {
		return fmt.Errorf("restart command failed: %w", err)
	}
	r.ui.Say(fmt.Sprintf("Waiting up to %s for the machine to restart...", r.cfg.RestartTimeout))
	if _, err := r.probeUntilRestarted(ctx, r.now().Add(r.cfg.RestartTimeout), before.BootTime); err != nil {
		r.addCheck("WinRM after restart", false, err.Error())
		return fmt.Errorf("the machine did not come back over WinRM within %s: %w", r.cfg.RestartTimeout, err)
	}
	r.reboot.Restarts++
	return r.remote.install(ctx)
}

// probeUntilRestarted repeats the Probe action until it reports a boot time
// other than previousBootTime twice in a row, at least the settle period
// apart, or until deadline passes. After a failed attempt the guest scripts
// are uploaded again before the next one, because the restart may have
// removed the working directory.
//
// Returns:
//   - *facts.Probe: The settled probe.
//   - error: The last failure when the deadline passes, or the context error.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *run) probeUntilRestarted(ctx context.Context, deadline time.Time, previousBootTime string) (*facts.Probe, error) {
	ctx, cancelDeadline := context.WithDeadline(ctx, deadline)
	defer cancelDeadline()
	var lastErr error
	candidate := ""
	var candidateAt time.Time
	for {
		attemptCtx, cancel := context.WithTimeout(ctx, probeAttemptTimeout)
		var p *facts.Probe
		var err error
		if lastErr != nil {
			err = r.remote.install(attemptCtx)
		}
		if err == nil {
			p, err = r.probe(attemptCtx)
		}
		cancel()
		wait := r.timing.probeInterval
		switch {
		case err != nil:
			lastErr, candidate = err, ""
		case p.BootTime == previousBootTime:
			lastErr = errors.New("the machine has not restarted yet")
		case p.BootTime == candidate && !r.now().Before(candidateAt.Add(r.timing.settle)):
			return p, nil
		case p.BootTime == candidate:
			wait = candidateAt.Add(r.timing.settle).Sub(r.now())
		default:
			candidate, candidateAt, lastErr = p.BootTime, r.now(), nil
			r.ui.Say("The machine is back; waiting for feature configuration to settle...")
			wait = r.timing.settle
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !r.now().Add(wait).Before(deadline) {
			if lastErr == nil {
				lastErr = errors.New("the machine did not stay up through the settle period")
			}
			return nil, lastErr
		}
		if err := sleepFor(ctx, wait); err != nil {
			return nil, err
		}
	}
}

// sleepFor waits for d, or until ctx is done.
//
// Returns:
//   - error: The context error, or nil.
//
// Author: Lars Krogh
// Version: 1.0.0
func sleepFor(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
