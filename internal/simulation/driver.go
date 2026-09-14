package simulation

import (
	"context"
	"crypto/rand"
	"errors"
	"sync/atomic"
	"time"

	"github.com/miroslav-matejovsky/ais-test-bench/simulation"
)

// Driver paces one public simulation engine with wall-clock ticks and supplies
// the current real time to fleet mutations. It holds no simulation state; all
// generation, history, and metadata come from the engine.
type Driver struct {
	sim     *simulation.Simulator
	started atomic.Bool
}

// NewID returns a random opaque simulation identity. Call it once per engine
// start so runs using the same seed remain distinguishable.
func NewID() string {
	return rand.Text()
}

// NewDriver returns a driver for sim. The driver must be the only caller of
// sim's mutating methods.
func NewDriver(sim *simulation.Simulator) *Driver {
	return &Driver{sim: sim}
}

// Run advances the engine every simulation.TickInterval until cancellation or
// an encoding error. It may be called once per driver; a second call returns an
// error. The simplified reporting cadence is intended for live UI development.
func (d *Driver) Run(ctx context.Context) error {
	if !d.started.CompareAndSwap(false, true) {
		return errors.New("simulation driver already started")
	}
	ticker := time.NewTicker(simulation.TickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-ticker.C:
			if err := d.sim.Advance(now); err != nil {
				return err
			}
		}
	}
}

// SetCount sets the active fleet size at the current real time. See
// simulation.Simulator.SetCount.
func (d *Driver) SetCount(count int) error {
	return d.sim.SetCount(count, time.Now())
}

// Fleet returns a copy of the active fleet.
func (d *Driver) Fleet() simulation.Fleet {
	return d.sim.Fleet()
}

// History returns a copy of the retained reports.
func (d *Driver) History() simulation.History {
	return d.sim.History()
}

// Metadata returns the run identity, catalogs, and effective settings.
func (d *Driver) Metadata() simulation.Metadata {
	return d.sim.Metadata()
}
