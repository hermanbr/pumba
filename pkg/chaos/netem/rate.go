package netem

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"

	"github.com/alexei-led/pumba/pkg/chaos"
	"github.com/alexei-led/pumba/pkg/container"
	log "github.com/sirupsen/logrus"
)

// Parse rate
func parseRate(rate string) (string, error) {
	reRate := regexp.MustCompile(`\d+[gmk]?bit`)
	validRate := reRate.FindString(rate)
	if rate != validRate {
		return "", fmt.Errorf("invalid rate, must match '%s'", reRate.String())
	}
	return rate, nil
}

// `netem rate` command
type rateCommand struct {
	client         netemClient
	gp             *chaos.GlobalParams
	req            *container.NetemRequest
	limit          int
	rate           string
	packetOverhead int
	cellSize       int
	cellOverhead   int
}

// validateRateParams checks netem rate parameters; shared by the rate and chained-effects commands.
func validateRateParams(rate string, cellSize int) error {
	// validate target egress rate
	if rate == "" {
		return errors.New("undefined rate limit")
	}
	if _, err := parseRate(rate); err != nil {
		return fmt.Errorf("invalid rate: %w", err)
	}

	// validate cell size
	if cellSize < 0 {
		return errors.New("invalid cell size: must be a non-negative integer")
	}
	return nil
}

// rateArgs builds the netem 'rate ...' argument list; shared by the rate and chained-effects commands.
func rateArgs(rate string, packetOverhead, cellSize, cellOverhead int) []string {
	cmd := []string{"rate", rate}
	if packetOverhead != 0 {
		cmd = append(cmd, strconv.Itoa(packetOverhead))
	}
	if cellSize > 0 {
		cmd = append(cmd, strconv.Itoa(cellSize))
	}
	if cellOverhead != 0 {
		cmd = append(cmd, strconv.Itoa(cellOverhead))
	}
	return cmd
}

// NewRateCommand create new netem rate command
func NewRateCommand(client netemClient,
	gp *chaos.GlobalParams,
	req *container.NetemRequest,
	limit int,
	rate string, // delay outgoing packets; in common units
	packetOverhead, // per packet overhead; in bytes
	cellSize, // cell size of the simulated link layer scheme
	cellOverhead int, // per cell overhead; in bytes
) (chaos.Command, error) {
	if err := validateRateParams(rate, cellSize); err != nil {
		return nil, err
	}

	return &rateCommand{
		client:         client,
		gp:             gp,
		req:            req,
		limit:          limit,
		rate:           rate,
		packetOverhead: packetOverhead,
		cellSize:       cellSize,
		cellOverhead:   cellOverhead,
	}, nil
}

// Run netem rate command
func (n *rateCommand) Run(ctx context.Context, random bool) error {
	log.Debug("setting network rate to all matching containers")
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
			log.WithFields(log.Fields{
				"container": c,
				"command":   netemCmd,
			}).Debug("setting network rate for container")
			netemCtx, cancel := context.WithTimeout(ctx, n.req.Duration)
			defer cancel()
			req := *n.req
			req.Container = c
			req.Command = netemCmd
			if err := runNetem(netemCtx, n.client, &req); err != nil {
				log.WithError(err).Warn("failed to set network rate for container")
				return fmt.Errorf("failed to set network rate for one or more containers: %w", err)
			}
			return nil
		})
}

func (n *rateCommand) buildNetemCmd() []string {
	return rateArgs(n.rate, n.packetOverhead, n.cellSize, n.cellOverhead)
}
