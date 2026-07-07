package netem

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/alexei-led/pumba/pkg/chaos"
	"github.com/alexei-led/pumba/pkg/container"
	log "github.com/sirupsen/logrus"
)

// `netem loss` command
type lossCommand struct {
	client      netemClient
	gp          *chaos.GlobalParams
	req         *container.NetemRequest
	limit       int
	percent     float64
	correlation float64
}

// validateLoss checks netem loss parameters; shared by the loss and chained-effects commands.
func validateLoss(percent, correlation float64) error {
	// get netem loss percent
	if percent < 0.0 || percent > 100.0 {
		return errors.New("invalid loss percent: must be between 0.0 and 100.0")
	}
	// get netem loss variation
	if correlation < 0.0 || correlation > 100.0 {
		return errors.New("invalid loss correlation: must be between 0.0 and 100.0")
	}
	return nil
}

// lossArgs builds the netem 'loss ...' argument list; shared by the loss and chained-effects commands.
func lossArgs(percent, correlation float64) []string {
	cmd := []string{"loss", strconv.FormatFloat(percent, 'f', 2, 64)}
	if correlation > 0 {
		cmd = append(cmd, strconv.FormatFloat(correlation, 'f', 2, 64))
	}
	return cmd
}

// NewLossCommand create new netem loss command
func NewLossCommand(client netemClient,
	gp *chaos.GlobalParams,
	req *container.NetemRequest,
	limit int,
	percent, // loss percent
	correlation float64, // loss correlation
) (chaos.Command, error) {
	if err := validateLoss(percent, correlation); err != nil {
		return nil, err
	}

	return &lossCommand{
		client:      client,
		gp:          gp,
		req:         req,
		limit:       limit,
		percent:     percent,
		correlation: correlation,
	}, nil
}

// Run netem loss command
func (n *lossCommand) Run(ctx context.Context, random bool) error {
	log.Debug("adding network random packet loss to all matching containers")
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
			log.WithFields(log.Fields{"container": *c}).Debug("adding network random packet loss for container")
			netemCtx, cancel := context.WithTimeout(ctx, n.req.Duration)
			defer cancel()
			req := *n.req
			req.Container = c
			req.Command = netemCmd
			if err := runNetem(netemCtx, n.client, &req); err != nil {
				log.WithError(err).Warn("failed to set packet loss for container")
				return fmt.Errorf("failed to add packet loss for one or more containers: %w", err)
			}
			return nil
		})
}

func (n *lossCommand) buildNetemCmd() []string {
	return lossArgs(n.percent, n.correlation)
}
