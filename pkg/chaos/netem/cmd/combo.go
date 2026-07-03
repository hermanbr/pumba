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

// ComboParams holds the per-command parameters for the netem combo subcommand.
type ComboParams struct {
	Base  *container.NetemRequest
	Limit int
	Spec  netem.ComboSpec
}

// NewComboCLICommand initialize CLI combo command.
func NewComboCLICommand(ctx context.Context, runtime chaos.Runtime) *cli.Command {
	return chaoscmd.NewAction(ctx, runtime, chaoscmd.Spec[ComboParams]{
		Name: "combo",
		Flags: []cli.Flag{
			cli.IntFlag{
				Name:  "delay",
				Usage: "enable delay effect: delay time; in milliseconds (0: disabled)",
			},
			cli.IntFlag{
				Name:  "delay-jitter",
				Usage: "random delay variation (jitter); in milliseconds",
			},
			cli.Float64Flag{
				Name:  "delay-correlation",
				Usage: "delay correlation; in percentage",
			},
			cli.StringFlag{
				Name:  "delay-distribution",
				Usage: "delay distribution, can be one of {<empty> | uniform | normal | pareto |  paretonormal}",
			},
			cli.Float64Flag{
				Name:  "loss",
				Usage: "enable loss effect: packet loss percentage (0: disabled)",
			},
			cli.Float64Flag{
				Name:  "loss-correlation",
				Usage: "loss correlation; in percentage",
			},
			cli.Float64Flag{
				Name:  "corrupt",
				Usage: "enable corrupt effect: packet corrupt percentage (0: disabled)",
			},
			cli.Float64Flag{
				Name:  "corrupt-correlation",
				Usage: "corrupt correlation; in percentage",
			},
			cli.Float64Flag{
				Name:  "duplicate",
				Usage: "enable duplicate effect: packet duplicate percentage (0: disabled)",
			},
			cli.Float64Flag{
				Name:  "duplicate-correlation",
				Usage: "duplicate correlation; in percentage",
			},
			cli.StringFlag{
				Name:  "rate",
				Usage: "enable rate effect: egress rate limit; in common units, e.g. '100kbit' (empty: disabled)",
			},
			cli.IntFlag{
				Name:  "rate-packet-overhead",
				Usage: "rate: per packet overhead; in bytes",
			},
			cli.IntFlag{
				Name:  "rate-cell-size",
				Usage: "rate: cell size of the simulated link layer scheme",
			},
			cli.IntFlag{
				Name:  "rate-cell-overhead",
				Usage: "rate: per cell overhead; in bytes",
			},
		},
		Usage:       "combine multiple netem effects in a single qdisc",
		ArgsUsage:   fmt.Sprintf("containers (name, list of names, or RE2 regex if prefixed with %q", chaos.Re2Prefix),
		Description: "apply several netem effects (delay, loss, duplicate, corrupt, rate) at once to egress traffic of specified containers; effects are combined into a single 'tc netem' qdisc; enable an effect by setting its primary flag",
		Parse:       parseComboParams,
		Build:       buildComboCommand,
	})
}

func parseComboParams(c cliflags.Flags, gp *chaos.GlobalParams) (ComboParams, error) {
	base, limit, err := netem.ParseRequestBase(c.Parent(), gp)
	if err != nil {
		return ComboParams{}, fmt.Errorf("error parsing netem parameters: %w", err)
	}
	spec := netem.ComboSpec{}
	if c.Int("delay") > 0 {
		spec.Delay = &netem.DelaySpec{
			Time:         c.Int("delay"),
			Jitter:       c.Int("delay-jitter"),
			Correlation:  c.Float64("delay-correlation"),
			Distribution: c.String("delay-distribution"),
		}
	}
	if c.Float64("loss") > 0 {
		spec.Loss = &netem.LossSpec{
			Percent:     c.Float64("loss"),
			Correlation: c.Float64("loss-correlation"),
		}
	}
	if c.Float64("corrupt") > 0 {
		spec.Corrupt = &netem.CorruptSpec{
			Percent:     c.Float64("corrupt"),
			Correlation: c.Float64("corrupt-correlation"),
		}
	}
	if c.Float64("duplicate") > 0 {
		spec.Duplicate = &netem.DuplicateSpec{
			Percent:     c.Float64("duplicate"),
			Correlation: c.Float64("duplicate-correlation"),
		}
	}
	if c.String("rate") != "" {
		spec.Rate = &netem.RateSpec{
			Rate:           c.String("rate"),
			PacketOverhead: c.Int("rate-packet-overhead"),
			CellSize:       c.Int("rate-cell-size"),
			CellOverhead:   c.Int("rate-cell-overhead"),
		}
	}
	return ComboParams{
		Base:  base,
		Limit: limit,
		Spec:  spec,
	}, nil
}

func buildComboCommand(client container.Client, gp *chaos.GlobalParams, p ComboParams) (chaos.Command, error) {
	return netem.NewComboCommand(client, gp, p.Base, p.Limit, p.Spec)
}
