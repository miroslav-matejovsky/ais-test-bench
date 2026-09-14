package simulation

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	mathrand "math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/miroslav-matejovsky/ais-test-bench/simulation"
)

const (
	// Heartbeat is the real interval between elapsed-time deliveries.
	Heartbeat = 100 * time.Millisecond
	// MaxCatchUp is the largest virtual backlog one settlement replays.
	MaxCatchUp = time.Hour
	// elapseChunk bounds one Elapse call so its scaled duration stays within
	// simulation.MaxAdvance even at simulation.MaxSpeed.
	elapseChunk = simulation.MaxAdvance / time.Duration(simulation.MaxSpeed)
)

// Clock is the real-time source of a Driver.
type Clock interface {
	// Now returns the current real instant. Real clocks keep the monotonic
	// reading so elapsed time ignores wall-clock adjustments.
	Now() time.Time
	// NewTicker returns a channel that receives a wakeup about every interval
	// and a function that releases it.
	NewTicker(interval time.Duration) (<-chan time.Time, func())
}

// SystemClock is the operating system clock.
type SystemClock struct{}

// Now returns time.Now.
func (SystemClock) Now() time.Time { return time.Now() }

// NewTicker wraps time.NewTicker.
func (SystemClock) NewTicker(interval time.Duration) (<-chan time.Time, func()) {
	ticker := time.NewTicker(interval)
	return ticker.C, ticker.Stop
}

// Driver paces one public simulation engine with measured real time and
// serializes count and speed commands with that pacing. It holds no simulation
// state; all generation, history, and metadata come from the engine.
type Driver struct {
	sim     *simulation.Simulator
	clock   Clock
	started atomic.Bool

	mu       sync.Mutex // Serializes settlement and commands.
	baseline time.Time  // Real instant up to which elapsed time was delivered.
}

// NewConfig returns the application's engine configuration: a fresh random
// identity and seed, so runs are distinguishable, the current real instant as
// the virtual start, one vessel, and real-time speed.
func NewConfig() simulation.Config {
	return simulation.Config{
		ID: rand.Text(), StartTime: time.Now(), Seed: mathrand.Uint64(),
		InitialVesselCount: 1, Speed: 1,
	}
}

// NewDriver returns a driver for sim on clock, using the current real instant
// as baseline. Create it right after the engine so construction time is not
// replayed. The driver must be the only caller of sim's mutating methods.
func NewDriver(sim *simulation.Simulator, clock Clock) *Driver {
	return &Driver{sim: sim, clock: clock, baseline: clock.Now()}
}

// Run settles elapsed real time on every Heartbeat until ctx is cancelled or
// settlement fails, for example after a backlog above MaxCatchUp. Elapsed time is
// measured when a wakeup is processed, so late or coalesced wakeups lose no
// virtual time. It may be called once per driver; a second call returns an
// error.
func (d *Driver) Run(ctx context.Context) error {
	if !d.started.CompareAndSwap(false, true) {
		return errors.New("simulation driver already started")
	}
	ticks, stop := d.clock.NewTicker(Heartbeat)
	defer stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticks:
			if err := d.settle(ctx); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
		}
	}
}

// SetCount validates count, settles elapsed real time at the current speed, and
// sets the fleet size at the settled virtual instant. Invalid counts wrap
// simulation.ErrInvalid and change nothing. A settlement failure leaves the
// count unapplied but keeps delivered chunks.
func (d *Driver) SetCount(ctx context.Context, count int) error {
	if count < 0 || count > simulation.MaxVessels {
		return fmt.Errorf("%w: vessel count must be between 0 and %d: %d", simulation.ErrInvalid, simulation.MaxVessels, count)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.settleLocked(ctx); err != nil {
		return fmt.Errorf("settle before count change: %w", err)
	}
	if _, err := d.sim.SetCount(count); err != nil {
		return fmt.Errorf("set vessel count: %w", err)
	}
	return nil
}

// SetSpeed validates speed, settles elapsed real time at the previous speed, and
// applies the new speed to later elapsed time. Invalid speeds wrap
// simulation.ErrInvalid and change nothing. A settlement failure leaves the speed
// unapplied but keeps delivered chunks.
func (d *Driver) SetSpeed(ctx context.Context, speed float64) error {
	if err := simulation.ValidateSpeed(speed); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.settleLocked(ctx); err != nil {
		return fmt.Errorf("settle before speed change: %w", err)
	}
	if err := d.sim.SetSpeed(speed); err != nil {
		return fmt.Errorf("set simulation speed: %w", err)
	}
	return nil
}

// settle delivers elapsed real time under the driver lock.
func (d *Driver) settle(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.settleLocked(ctx)
}

// settleLocked samples the clock and delivers the real time since the baseline
// to the engine in chunks, advancing the baseline after each delivered chunk.
// A sample before the baseline changes nothing. While paused the elapsed time is
// dropped. A backlog above MaxCatchUp virtual time fails before any delivery.
// Called with d.mu held.
func (d *Driver) settleLocked(ctx context.Context) error {
	now := d.clock.Now()
	elapsed := now.Sub(d.baseline)
	if elapsed <= 0 {
		return nil
	}
	clock := d.sim.Metadata().Time
	if clock.Paused {
		d.baseline = now
		return nil
	}
	if float64(elapsed)*clock.Speed > float64(MaxCatchUp) {
		return fmt.Errorf("simulation is %v of real time behind at %vx, beyond the catch-up limit of %v virtual time", elapsed, clock.Speed, MaxCatchUp)
	}
	for elapsed > 0 {
		chunk := min(elapsed, elapseChunk)
		if _, err := d.sim.Elapse(ctx, chunk); err != nil {
			return fmt.Errorf("deliver elapsed time: %w", err)
		}
		d.baseline = d.baseline.Add(chunk)
		elapsed -= chunk
	}
	return nil
}

// Fleet returns a copy of the active fleet without settling elapsed time.
func (d *Driver) Fleet() simulation.Fleet {
	return d.sim.Fleet()
}

// History returns a copy of the retained reports without settling elapsed time.
func (d *Driver) History() simulation.History {
	return d.sim.History()
}

// Metadata returns the run identity, committed clock, catalogs, and effective
// settings without settling elapsed time.
func (d *Driver) Metadata() simulation.Metadata {
	return d.sim.Metadata()
}
