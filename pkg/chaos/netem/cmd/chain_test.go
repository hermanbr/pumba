package cmd

import (
	"flag"
	"testing"

	"github.com/alexei-led/pumba/pkg/chaos"
	"github.com/alexei-led/pumba/pkg/chaos/cliflags"
	"github.com/alexei-led/pumba/pkg/chaos/netem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

// effectCtx builds a cliflags.Flags over the given effect flag definitions,
// mirroring how urfave/cli hands a subcommand its parsed context.
func effectCtx(t *testing.T, flags []cli.Flag, args []string) cliflags.Flags {
	t.Helper()
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	for _, fl := range flags {
		fl.Apply(fs)
	}
	require.NoError(t, fs.Parse(args))
	return cliflags.NewV1(cli.NewContext(nil, fs, nil))
}

func TestParseChain_SingleSegment(t *testing.T) {
	spec := &netem.EffectsSpec{}
	rest, err := parseChain([]string{"loss", "--percent", "20", "mydb"}, spec)
	require.NoError(t, err)
	assert.Equal(t, []string{"mydb"}, rest)
	require.NotNil(t, spec.Loss)
	assert.InEpsilon(t, 20.0, spec.Loss.Percent, 1e-9)
}

func TestParseChain_MultipleSegmentsWithAliases(t *testing.T) {
	spec := &netem.EffectsSpec{}
	rest, err := parseChain([]string{"loss", "-p", "15", "corrupt", "--percent", "5", "-c", "10", "c1", "c2"}, spec)
	require.NoError(t, err)
	assert.Equal(t, []string{"c1", "c2"}, rest)
	require.NotNil(t, spec.Loss)
	assert.InEpsilon(t, 15.0, spec.Loss.Percent, 1e-9)
	require.NotNil(t, spec.Corrupt)
	assert.InEpsilon(t, 5.0, spec.Corrupt.Percent, 1e-9)
	assert.InEpsilon(t, 10.0, spec.Corrupt.Correlation, 1e-9)
}

func TestParseChain_SegmentKeepsSubcommandDefaults(t *testing.T) {
	spec := &netem.EffectsSpec{}
	rest, err := parseChain([]string{"delay", "rate", "mydb"}, spec)
	require.NoError(t, err)
	assert.Equal(t, []string{"mydb"}, rest)
	// delay segment without flags gets the delay subcommand's own defaults
	require.NotNil(t, spec.Delay)
	assert.Equal(t, 100, spec.Delay.Time)
	assert.Equal(t, 10, spec.Delay.Jitter)
	assert.InEpsilon(t, 20.0, spec.Delay.Correlation, 1e-9)
	// rate segment defaults to the rate subcommand's default rate
	require.NotNil(t, spec.Rate)
	assert.Equal(t, "100kbit", spec.Rate.Rate)
}

func TestParseChain_NoEffectKeyword(t *testing.T) {
	spec := &netem.EffectsSpec{}
	rest, err := parseChain([]string{"mydb", "other"}, spec)
	require.NoError(t, err)
	assert.Equal(t, []string{"mydb", "other"}, rest)
	assert.Equal(t, netem.EffectsSpec{}, *spec)
}

func TestParseChain_AllArgsConsumed(t *testing.T) {
	spec := &netem.EffectsSpec{}
	rest, err := parseChain([]string{"loss", "--percent", "20"}, spec)
	require.NoError(t, err)
	assert.Empty(t, rest, "no leftover args means label / match-all targeting")
	require.NotNil(t, spec.Loss)
}

func TestParseChain_DuplicateEffectRejected(t *testing.T) {
	spec := &netem.EffectsSpec{Loss: &netem.LossSpec{Percent: 10}}
	_, err := parseChain([]string{"loss", "--percent", "20", "mydb"}, spec)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `duplicate netem effect "loss"`)
}

func TestParseChain_LossModelsNotChainable(t *testing.T) {
	for _, name := range []string{"loss-state", "loss-gemodel"} {
		spec := &netem.EffectsSpec{}
		_, err := parseChain([]string{name, "--p13", "10", "mydb"}, spec)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be chained")
	}
}

func TestParseChain_BadFlagNamesSegment(t *testing.T) {
	spec := &netem.EffectsSpec{}
	_, err := parseChain([]string{"loss", "--bogus", "20", "mydb"}, spec)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `chained netem effect "loss"`)
}

func TestChainSpec_NoChainReturnsNotFound(t *testing.T) {
	c := effectCtx(t, delayFlags(), []string{"--time", "200", "mydb"})
	gp := &chaos.GlobalParams{Names: []string{"mydb"}}
	_, found, err := chainSpec(c, gp, applyDelay)
	require.NoError(t, err)
	assert.False(t, found, "single-effect invocation must use the original code path")
	assert.Equal(t, []string{"mydb"}, gp.Names, "gp untouched without a chain")
}

func TestChainSpec_ChainCombinesPrimaryAndSegments(t *testing.T) {
	c := effectCtx(t, delayFlags(), []string{"--time", "200", "loss", "--percent", "15", "mydb"})
	// NewAction parsed names from raw args — chainSpec must fix them up
	gp := &chaos.GlobalParams{Names: []string{"loss", "--percent", "15", "mydb"}}
	spec, found, err := chainSpec(c, gp, applyDelay)
	require.NoError(t, err)
	require.True(t, found)
	require.NotNil(t, spec.Delay)
	assert.Equal(t, 200, spec.Delay.Time)
	require.NotNil(t, spec.Loss)
	assert.InEpsilon(t, 15.0, spec.Loss.Percent, 1e-9)
	assert.Equal(t, []string{"mydb"}, gp.Names, "targets re-derived from leftover args")
	assert.Empty(t, gp.Pattern)
}

func TestChainSpec_Re2PatternLeftover(t *testing.T) {
	c := effectCtx(t, lossFlags(), []string{"--percent", "10", "corrupt", "--percent", "5", "re2:^db"})
	gp := &chaos.GlobalParams{}
	_, found, err := chainSpec(c, gp, applyLoss)
	require.NoError(t, err)
	require.True(t, found)
	assert.Empty(t, gp.Names)
	assert.Equal(t, "^db", gp.Pattern)
}

func TestChainSpec_DuplicateOfPrimaryRejected(t *testing.T) {
	c := effectCtx(t, lossFlags(), []string{"--percent", "10", "loss", "--percent", "20", "mydb"})
	gp := &chaos.GlobalParams{}
	_, _, err := chainSpec(c, gp, applyLoss)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `duplicate netem effect "loss"`)
}
