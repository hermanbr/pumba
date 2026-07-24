package netem

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/alexei-led/pumba/pkg/chaos"
	"github.com/alexei-led/pumba/pkg/container"
	log "github.com/sirupsen/logrus"
)

var (
	// DelayDistribution netem delay distributions
	delayDistribution = []string{"", "uniform", "normal", "pareto", "paretonormal"}
)

// delayQueueLimit is the netem qdisc queue length (in packets) applied whenever
// a delay effect is present. netem's kernel default is only 1000, which tail-drops
// (adding unwanted loss + a throughput cap) once the bandwidth-delay product
// exceeds it — e.g. ~8300 packets are in flight at 1 Gbps × 100 ms. 10000 gives
// headroom for 1 Gbps sub-second delay at a negligible ~15 MB max buffer.
const delayQueueLimit = 10000

// `netem delay` command
type delayCommand struct {
	client       netemClient
	gp           *chaos.GlobalParams
	req          *container.NetemRequest
	limit        int
	time         int
	jitter       int
	correlation  float64
	distribution string
}

// validateDelay checks netem delay parameters; shared by the delay and chained-effects commands.
func validateDelay(delay, jitter int, correlation float64, distribution string) error {
	// check delay time
	if delay <= 0 {
		return errors.New("non-positive delay time")
	}
	// get delay variation
	if jitter < 0 || jitter > delay {
		return errors.New("invalid delay jitter: must be non-negative and smaller than delay time")
	}
	// get delay variation
	if correlation < 0.0 || correlation > 100.0 {
		return errors.New("invalid delay correlation: must be between 0.0 and 100.0")
	}
	// get distribution
	if !slices.Contains(delayDistribution, distribution) {
		return errors.New("invalid delay distribution: must be one of {uniform | normal | pareto |  paretonormal}")
	}
	return nil
}

// delayArgs builds the netem 'delay ...' argument list; shared by the delay and chained-effects commands.
func delayArgs(delay, jitter int, correlation float64, distribution string) []string {
	cmd := []string{"delay", strconv.Itoa(delay) + "ms"}
	if jitter > 0 {
		cmd = append(cmd, strconv.Itoa(jitter)+"ms")
	}
	if correlation > 0 {
		cmd = append(cmd, strconv.FormatFloat(correlation, 'f', 2, 64))
	}
	if distribution != "" {
		cmd = append(cmd, "distribution", distribution)
	}
	// set the qdisc queue length explicitly whenever delay is applied
	cmd = append(cmd, "limit", strconv.Itoa(delayQueueLimit))
	return cmd
}

// NewDelayCommand create new netem delay command
func NewDelayCommand(client netemClient,
	gp *chaos.GlobalParams,
	req *container.NetemRequest,
	limit int,
	delay, // delay time
	jitter int, // delay jitter
	correlation float64, // delay correlation
	distribution string, // delay distribution
) (chaos.Command, error) {
	if err := validateDelay(delay, jitter, correlation, distribution); err != nil {
		return nil, err
	}
	return &delayCommand{
		client:       client,
		gp:           gp,
		req:          req,
		limit:        limit,
		time:         delay,
		jitter:       jitter,
		correlation:  correlation,
		distribution: distribution,
	}, nil
}

// Run netem delay command
func (n *delayCommand) Run(ctx context.Context, random bool) error {
	log.Debug("adding network delay to all matching containers")
	log.WithFields(log.Fields{
		"names":   n.gp.Names,
		"pattern": n.gp.Pattern,
		"labels":  n.gp.Labels,
		"limit":   n.limit,
		"random":  random,
	}).Debug("listing matching containers")
	netemCmd := n.buildNetemCmd()
	return chaos.RunOnContainers(ctx, n.client, n.gp, n.limit, random, true,
		func(ctx context.Context, c *container.Container) error {
			log.WithFields(log.Fields{"container": c}).Debug("adding network delay for container")
			netemCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			req := *n.req
			req.Container = c
			req.Command = netemCmd
			if err := runNetem(netemCtx, n.client, &req); err != nil {
				log.WithError(err).Warn("failed to delay network for container")
				return fmt.Errorf("failed to delay packets for one or more containers: %w", err)
			}
			return nil
		})
}

func (n *delayCommand) buildNetemCmd() []string {
	return delayArgs(n.time, n.jitter, n.correlation, n.distribution)
}
