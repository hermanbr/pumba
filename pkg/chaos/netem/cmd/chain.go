package cmd

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/alexei-led/pumba/pkg/chaos"
	"github.com/alexei-led/pumba/pkg/chaos/cliflags"
	"github.com/alexei-led/pumba/pkg/chaos/netem"
	"github.com/urfave/cli"
)

// effectDef describes a chainable netem effect: its subcommand name, its flag
// definitions (the same list used by the CLI subcommand, so names, aliases and
// defaults stay identical), and how a parsed segment is applied onto the
// combined EffectsSpec.
type effectDef struct {
	name  string
	flags func() []cli.Flag
	apply func(f cliflags.Flags, spec *netem.EffectsSpec) error
}

// netemEffects registers every chainable netem effect keyword.
func netemEffects() []effectDef {
	return []effectDef{
		{name: "delay", flags: delayFlags, apply: applyDelay},
		{name: "loss", flags: lossFlags, apply: applyLoss},
		{name: "corrupt", flags: corruptFlags, apply: applyCorrupt},
		{name: "duplicate", flags: duplicateFlags, apply: applyDuplicate},
		{name: "rate", flags: rateFlags, apply: applyRate},
	}
}

// lossModelNames are netem subcommands that are recognized as effect keywords
// but cannot be chained: loss-state and loss-gemodel are alternative loss
// models, mutually exclusive with random loss and with each other on a single
// netem qdisc.
func isNonChainableEffect(name string) bool {
	return name == "loss-state" || name == "loss-gemodel"
}

func effectByName(name string) (effectDef, bool) {
	for _, def := range netemEffects() {
		if def.name == name {
			return def, true
		}
	}
	return effectDef{}, false
}

func applyDelay(f cliflags.Flags, spec *netem.EffectsSpec) error {
	if spec.Delay != nil {
		return fmt.Errorf("duplicate netem effect %q in chain", "delay")
	}
	spec.Delay = &netem.DelaySpec{
		Time:         f.Int("time"),
		Jitter:       f.Int("jitter"),
		Correlation:  f.Float64("correlation"),
		Distribution: f.String("distribution"),
	}
	return nil
}

func applyLoss(f cliflags.Flags, spec *netem.EffectsSpec) error {
	if spec.Loss != nil {
		return fmt.Errorf("duplicate netem effect %q in chain", "loss")
	}
	spec.Loss = &netem.LossSpec{
		Percent:     f.Float64("percent"),
		Correlation: f.Float64("correlation"),
	}
	return nil
}

func applyCorrupt(f cliflags.Flags, spec *netem.EffectsSpec) error {
	if spec.Corrupt != nil {
		return fmt.Errorf("duplicate netem effect %q in chain", "corrupt")
	}
	spec.Corrupt = &netem.CorruptSpec{
		Percent:     f.Float64("percent"),
		Correlation: f.Float64("correlation"),
	}
	return nil
}

func applyDuplicate(f cliflags.Flags, spec *netem.EffectsSpec) error {
	if spec.Duplicate != nil {
		return fmt.Errorf("duplicate netem effect %q in chain", "duplicate")
	}
	spec.Duplicate = &netem.DuplicateSpec{
		Percent:     f.Float64("percent"),
		Correlation: f.Float64("correlation"),
	}
	return nil
}

func applyRate(f cliflags.Flags, spec *netem.EffectsSpec) error {
	if spec.Rate != nil {
		return fmt.Errorf("duplicate netem effect %q in chain", "rate")
	}
	spec.Rate = &netem.RateSpec{
		Rate:           f.String("rate"),
		PacketOverhead: f.Int("packetoverhead"),
		CellSize:       f.Int("cellsize"),
		CellOverhead:   f.Int("celloverhead"),
	}
	return nil
}

// normalizeAliases mirrors urfave/cli v1's unexported normalizeFlags: cli
// registers every alias of a flag ("percent, p") as a separate Go flag, and
// after parsing copies the explicitly-set alias's value onto the flag's other
// names so lookups by canonical name see it. Required here because segments
// are parsed with a raw flag.FlagSet instead of going through cli.Command.Run.
func normalizeAliases(flags []cli.Flag, fs *flag.FlagSet) {
	visited := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { visited[f.Name] = true })
	for _, fl := range flags {
		parts := strings.Split(fl.GetName(), ",")
		if len(parts) == 1 {
			continue
		}
		names := make([]string, 0, len(parts))
		for _, p := range parts {
			names = append(names, strings.TrimSpace(p))
		}
		setName := ""
		for _, n := range names {
			if visited[n] {
				setName = n
				break
			}
		}
		if setName == "" {
			continue
		}
		value := fs.Lookup(setName).Value.String()
		for _, n := range names {
			if n != setName {
				_ = fs.Set(n, value) // same flag definition: value is valid for every alias
			}
		}
	}
}

// chainSpec detects chained effect segments in c.Args(). It reports found ==
// false when the invocation is a plain single-effect command (leaving the
// original code path untouched). When the first positional arg is another
// effect keyword, it applies the primary effect (already parsed from c) plus
// every chained segment onto a fresh EffectsSpec and re-derives the target
// names/pattern in gp from the leftover args.
func chainSpec(c cliflags.Flags, gp *chaos.GlobalParams, primary func(cliflags.Flags, *netem.EffectsSpec) error) (spec netem.EffectsSpec, found bool, err error) {
	args := c.Args()
	if len(args) == 0 {
		return spec, false, nil
	}
	if _, chainable := effectByName(args[0]); !chainable && !isNonChainableEffect(args[0]) {
		return spec, false, nil
	}
	if applyErr := primary(c, &spec); applyErr != nil {
		return spec, false, applyErr
	}
	rest, err := parseChain(args, &spec)
	if err != nil {
		return spec, false, err
	}
	gp.Names, gp.Pattern = chaos.NamesOrPattern(rest)
	return spec, true, nil
}

// parseChain consumes effect segments from the front of args. Each segment is
// parsed with the effect's own cli.Flag definitions (same defaults and aliases
// as its subcommand); Go's flag parser stops at the first non-flag token, which
// either starts the next segment or the container names. Returns the leftover
// (non-effect) args.
func parseChain(args []string, spec *netem.EffectsSpec) ([]string, error) {
	for len(args) > 0 {
		if isNonChainableEffect(args[0]) {
			return nil, fmt.Errorf("netem effect %q cannot be chained: loss models are mutually exclusive on a single qdisc", args[0])
		}
		def, ok := effectByName(args[0])
		if !ok {
			return args, nil // container names / re2: pattern
		}
		fs := flag.NewFlagSet(def.name, flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		for _, fl := range def.flags() {
			fl.Apply(fs)
		}
		if err := fs.Parse(args[1:]); err != nil {
			return nil, fmt.Errorf("parsing chained netem effect %q: %w", def.name, err)
		}
		normalizeAliases(def.flags(), fs)
		if err := def.apply(cliflags.NewV1(cli.NewContext(nil, fs, nil)), spec); err != nil {
			return nil, err
		}
		args = fs.Args()
	}
	return nil, nil // all args consumed: no explicit targets (match-all / label mode)
}
