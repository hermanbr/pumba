//nolint:dupl // Generic NewAction[P] enforces a uniform per-command shape; the residual similarity is intentional, not copy-paste.
package cmd

import (
	"context"
	"fmt"

	"github.com/alexei-led/pumba/pkg/chaos"
	"github.com/alexei-led/pumba/pkg/chaos/cliflags"
	chaoscmd "github.com/alexei-led/pumba/pkg/chaos/cmd"
	"github.com/alexei-led/pumba/pkg/chaos/netem"
	"github.com/alexei-led/pumba/pkg/container"
	"github.com/urfave/cli"
)

// RateParams holds the per-command parameters for the netem rate subcommand.
type RateParams struct {
	Base           *container.NetemRequest
	Limit          int
	Rate           string
	PacketOverhead int
	CellSize       int
	CellOverhead   int
	Chain          *netem.EffectsSpec // set when additional effects are chained after this one
}

// rateFlags returns the rate subcommand's flag definitions; shared with the
// effect-chaining registry so chained segments parse identically.
func rateFlags() []cli.Flag {
	return []cli.Flag{
		cli.StringFlag{
			Name:  "rate, r",
			Usage: "delay outgoing packets; in common units",
			Value: "100kbit",
		},
		cli.IntFlag{
			Name:  "packetoverhead, p",
			Usage: "per packet overhead; in bytes",
			Value: 0,
		},
		cli.IntFlag{
			Name:  "cellsize, s",
			Usage: "cell size of the simulated link layer scheme",
			Value: 0,
		},
		cli.IntFlag{
			Name:  "celloverhead, c",
			Usage: "per cell overhead; in bytes",
			Value: 0,
		},
	}
}

// NewRateCLICommand initialize CLI rate command.
func NewRateCLICommand(ctx context.Context, runtime chaos.Runtime) *cli.Command {
	return chaoscmd.NewAction(ctx, runtime, chaoscmd.Spec[RateParams]{
		Name:        "rate",
		Flags:       rateFlags(),
		Usage:       "rate limit egress traffic",
		ArgsUsage:   fmt.Sprintf("containers (name, list of names, or RE2 regex if prefixed with %q", chaos.Re2Prefix),
		Description: "rate limit egress traffic for specified containers",
		Parse:       parseRateParams,
		Build:       buildRateCommand,
	})
}

func parseRateParams(c cliflags.Flags, gp *chaos.GlobalParams) (RateParams, error) {
	base, limit, err := netem.ParseRequestBase(c.Parent(), gp)
	if err != nil {
		return RateParams{}, fmt.Errorf("error parsing netem parameters: %w", err)
	}
	chain, chained, err := chainSpec(c, gp, applyRate)
	if err != nil {
		return RateParams{}, err
	}
	params := RateParams{
		Base:           base,
		Limit:          limit,
		Rate:           c.String("rate"),
		PacketOverhead: c.Int("packetoverhead"),
		CellSize:       c.Int("cellsize"),
		CellOverhead:   c.Int("celloverhead"),
	}
	if chained {
		params.Chain = &chain
	}
	return params, nil
}

func buildRateCommand(client container.Client, gp *chaos.GlobalParams, p RateParams) (chaos.Command, error) {
	if p.Chain != nil {
		return netem.NewEffectsCommand(client, gp, p.Base, p.Limit, *p.Chain)
	}
	return netem.NewRateCommand(client, gp, p.Base, p.Limit, p.Rate, p.PacketOverhead, p.CellSize, p.CellOverhead)
}
