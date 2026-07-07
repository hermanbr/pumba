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

// CorruptParams holds the per-command parameters for the netem corrupt subcommand.
type CorruptParams struct {
	Base        *container.NetemRequest
	Limit       int
	Percent     float64
	Correlation float64
	Chain       *netem.EffectsSpec // set when additional effects are chained after this one
}

// corruptFlags returns the corrupt subcommand's flag definitions; shared with
// the effect-chaining registry so chained segments parse identically.
func corruptFlags() []cli.Flag {
	return []cli.Flag{
		cli.Float64Flag{
			Name:  "percent, p",
			Usage: "packet corrupt percentage",
			Value: 0.0,
		},
		cli.Float64Flag{
			Name:  "correlation, c",
			Usage: "corrupt correlation; in percentage",
			Value: 0.0,
		},
	}
}

// NewCorruptCLICommand initialize CLI corrupt command.
func NewCorruptCLICommand(ctx context.Context, runtime chaos.Runtime) *cli.Command {
	return chaoscmd.NewAction(ctx, runtime, chaoscmd.Spec[CorruptParams]{
		Name:        "corrupt",
		Flags:       corruptFlags(),
		Usage:       "adds packet corruption",
		ArgsUsage:   fmt.Sprintf("containers (name, list of names, or RE2 regex if prefixed with %q", chaos.Re2Prefix),
		Description: "adds packet corruption, based on independent (Bernoulli) probability model\n \tsee:  http://www.voiptroubleshooter.com/indepth/burstloss.html",
		Parse:       parseCorruptParams,
		Build:       buildCorruptCommand,
	})
}

func parseCorruptParams(c cliflags.Flags, gp *chaos.GlobalParams) (CorruptParams, error) {
	base, limit, err := netem.ParseRequestBase(c.Parent(), gp)
	if err != nil {
		return CorruptParams{}, fmt.Errorf("error parsing netem parameters: %w", err)
	}
	chain, chained, err := chainSpec(c, gp, applyCorrupt)
	if err != nil {
		return CorruptParams{}, err
	}
	params := CorruptParams{
		Base:        base,
		Limit:       limit,
		Percent:     c.Float64("percent"),
		Correlation: c.Float64("correlation"),
	}
	if chained {
		params.Chain = &chain
	}
	return params, nil
}

func buildCorruptCommand(client container.Client, gp *chaos.GlobalParams, p CorruptParams) (chaos.Command, error) {
	if p.Chain != nil {
		return netem.NewEffectsCommand(client, gp, p.Base, p.Limit, *p.Chain)
	}
	return netem.NewCorruptCommand(client, gp, p.Base, p.Limit, p.Percent, p.Correlation)
}
