package netem

import (
	"context"
	"errors"
	"fmt"

	"github.com/alexei-led/pumba/pkg/chaos"
	"github.com/alexei-led/pumba/pkg/container"
	log "github.com/sirupsen/logrus"
)

// DelaySpec holds netem delay parameters for chained netem effects.
type DelaySpec struct {
	Time         int
	Jitter       int
	Correlation  float64
	Distribution string
}

// LossSpec holds netem random loss parameters for chained netem effects.
type LossSpec struct {
	Percent     float64
	Correlation float64
}

// CorruptSpec holds netem corrupt parameters for chained netem effects.
type CorruptSpec struct {
	Percent     float64
	Correlation float64
}

// DuplicateSpec holds netem duplicate parameters for chained netem effects.
type DuplicateSpec struct {
	Percent     float64
	Correlation float64
}

// RateSpec holds netem rate parameters for chained netem effects.
type RateSpec struct {
	Rate           string
	PacketOverhead int
	CellSize       int
	CellOverhead   int
}

// EffectsSpec selects which netem effects to combine into a single qdisc.
// A nil field means the effect is disabled; at least one must be set.
type EffectsSpec struct {
	Delay     *DelaySpec
	Loss      *LossSpec
	Corrupt   *CorruptSpec
	Duplicate *DuplicateSpec
	Rate      *RateSpec
}

// validate checks every enabled effect using the same rules as the
// corresponding single-effect command.
func (s EffectsSpec) validate() error {
	checks := s.effectChecks()
	if len(checks) == 0 {
		return errors.New("at least one netem effect required: delay, loss, corrupt, duplicate or rate")
	}
	for _, check := range checks {
		if err := check(); err != nil {
			return err
		}
	}
	return nil
}

// effectChecks returns one validation closure per enabled effect.
func (s EffectsSpec) effectChecks() []func() error {
	var checks []func() error
	if s.Delay != nil {
		checks = append(checks, func() error {
			return validateDelay(s.Delay.Time, s.Delay.Jitter, s.Delay.Correlation, s.Delay.Distribution)
		})
	}
	if s.Loss != nil {
		checks = append(checks, func() error { return validateLoss(s.Loss.Percent, s.Loss.Correlation) })
	}
	if s.Corrupt != nil {
		checks = append(checks, func() error { return validateCorrupt(s.Corrupt.Percent, s.Corrupt.Correlation) })
	}
	if s.Duplicate != nil {
		checks = append(checks, func() error { return validateDuplicate(s.Duplicate.Percent, s.Duplicate.Correlation) })
	}
	if s.Rate != nil {
		checks = append(checks, func() error { return validateRateParams(s.Rate.Rate, s.Rate.CellSize) })
	}
	return checks
}

// args concatenates the enabled effects into a single netem argument list,
// e.g. 'delay 100ms 10ms loss 20.00 corrupt 5.00'.
func (s EffectsSpec) args() []string {
	var cmd []string
	if s.Delay != nil {
		cmd = append(cmd, delayArgs(s.Delay.Time, s.Delay.Jitter, s.Delay.Correlation, s.Delay.Distribution)...)
	}
	if s.Loss != nil {
		cmd = append(cmd, lossArgs(s.Loss.Percent, s.Loss.Correlation)...)
	}
	if s.Duplicate != nil {
		cmd = append(cmd, duplicateArgs(s.Duplicate.Percent, s.Duplicate.Correlation)...)
	}
	if s.Corrupt != nil {
		cmd = append(cmd, corruptArgs(s.Corrupt.Percent, s.Corrupt.Correlation)...)
	}
	if s.Rate != nil {
		cmd = append(cmd, rateArgs(s.Rate.Rate, s.Rate.PacketOverhead, s.Rate.CellSize, s.Rate.CellOverhead)...)
	}
	return cmd
}

// chained netem effects command
type effectsCommand struct {
	client netemClient
	gp     *chaos.GlobalParams
	req    *container.NetemRequest
	limit  int
	spec   EffectsSpec
}

// NewEffectsCommand create new netem effects command combining multiple netem
// effects (delay, loss, duplicate, corrupt, rate) in a single qdisc
func NewEffectsCommand(client netemClient,
	gp *chaos.GlobalParams,
	req *container.NetemRequest,
	limit int,
	spec EffectsSpec, // enabled netem effects
) (chaos.Command, error) {
	if err := spec.validate(); err != nil {
		return nil, err
	}
	return &effectsCommand{
		client: client,
		gp:     gp,
		req:    req,
		limit:  limit,
		spec:   spec,
	}, nil
}

// Run chained netem effects
func (n *effectsCommand) Run(ctx context.Context, random bool) error {
	log.Debug("adding combined network effects to all matching containers")
	log.WithFields(log.Fields{
		"names":   n.gp.Names,
		"pattern": n.gp.Pattern,
		"labels":  n.gp.Labels,
		"limit":   n.limit,
		"random":  random,
	}).Debug("listing matching containers")
	netemCmd := n.spec.args()
	return chaos.RunOnContainers(ctx, n.client, n.gp, n.limit, random, true,
		func(ctx context.Context, c *container.Container) error {
			log.WithFields(log.Fields{
				"container": c,
				"command":   netemCmd,
			}).Debug("adding combined network effects for container")
			netemCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			req := *n.req
			req.Container = c
			req.Command = netemCmd
			if err := runNetem(netemCtx, n.client, &req); err != nil {
				log.WithError(err).Warn("failed to add combined network effects for container")
				return fmt.Errorf("failed to add combined network effects for one or more containers: %w", err)
			}
			return nil
		})
}
