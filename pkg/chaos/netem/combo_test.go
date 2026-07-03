package netem

import (
	"context"
	"testing"
	"time"

	"github.com/alexei-led/pumba/pkg/chaos"
	"github.com/alexei-led/pumba/pkg/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestNewComboCommand_Validation(t *testing.T) {
	tests := []struct {
		name    string
		spec    ComboSpec
		wantErr string
	}{
		{
			name:    "no effects rejected",
			spec:    ComboSpec{},
			wantErr: "at least one netem effect required",
		},
		{
			name: "valid single effect",
			spec: ComboSpec{Delay: &DelaySpec{Time: 100}},
		},
		{
			name: "valid all effects",
			spec: ComboSpec{
				Delay:     &DelaySpec{Time: 100, Jitter: 10, Correlation: 25.0, Distribution: "normal"},
				Loss:      &LossSpec{Percent: 20.0, Correlation: 10.0},
				Corrupt:   &CorruptSpec{Percent: 5.0},
				Duplicate: &DuplicateSpec{Percent: 3.0},
				Rate:      &RateSpec{Rate: "100kbit"},
			},
		},
		{
			name:    "invalid delay propagated",
			spec:    ComboSpec{Delay: &DelaySpec{Time: 100, Jitter: 200}},
			wantErr: "invalid delay jitter",
		},
		{
			name:    "invalid delay distribution propagated",
			spec:    ComboSpec{Delay: &DelaySpec{Time: 100, Distribution: "gaussian"}},
			wantErr: "invalid delay distribution",
		},
		{
			name:    "invalid loss propagated",
			spec:    ComboSpec{Loss: &LossSpec{Percent: 101.0}},
			wantErr: "invalid loss percent",
		},
		{
			name:    "invalid corrupt propagated",
			spec:    ComboSpec{Corrupt: &CorruptSpec{Percent: 5.0, Correlation: 101.0}},
			wantErr: "invalid corrupt correlation",
		},
		{
			name:    "invalid duplicate propagated",
			spec:    ComboSpec{Duplicate: &DuplicateSpec{Percent: -1.0}},
			wantErr: "invalid duplicate percent",
		},
		{
			name:    "invalid rate propagated",
			spec:    ComboSpec{Rate: &RateSpec{Rate: "notarate"}},
			wantErr: "invalid rate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, gParams, nParams := validationFixtures(t)
			cmd, err := NewComboCommand(client, gParams, nParams, 0, tt.spec)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, cmd)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, cmd)
			}
		})
	}
}

func TestComboCommand_Run_NoContainers(t *testing.T) {
	mockClient := container.NewMockClient(t)
	gparams := &chaos.GlobalParams{Names: []string{"nonexistent"}}
	nparams := &container.NetemRequest{Interface: "eth0", Duration: time.Second}

	mockClient.EXPECT().ListContainers(mock.Anything,
		mock.AnythingOfType("container.FilterFunc"),
		container.ListOpts{All: false, Labels: nil}).
		Return([]*container.Container{}, nil)

	cmd, err := NewComboCommand(mockClient, gparams, nparams, 0, ComboSpec{Delay: &DelaySpec{Time: 100}})
	require.NoError(t, err)

	err = cmd.Run(context.Background(), false)
	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}

func TestComboCommand_Run_WithRandom(t *testing.T) {
	mockClient := container.NewMockClient(t)
	c1 := &container.Container{ContainerID: "id1", ContainerName: "c1"}
	c2 := &container.Container{ContainerID: "id2", ContainerName: "c2"}

	gparams := &chaos.GlobalParams{Names: []string{"c1", "c2"}, DryRun: true}
	nparams := &container.NetemRequest{
		Interface: "eth0",
		Duration:  100 * time.Millisecond,
		Sidecar:   container.SidecarSpec{Image: "tc"},
		DryRun:    true,
	}

	mockClient.EXPECT().ListContainers(mock.Anything,
		mock.AnythingOfType("container.FilterFunc"),
		container.ListOpts{All: false, Labels: nil}).
		Return([]*container.Container{c1, c2}, nil)

	mockClient.EXPECT().NetemContainer(mock.Anything, mock.AnythingOfType("*container.NetemRequest")).Return(nil).Once()
	mockClient.EXPECT().StopNetemContainer(mock.Anything, mock.AnythingOfType("*container.NetemRequest")).Return(nil).Once()

	cmd, err := NewComboCommand(mockClient, gparams, nparams, 0,
		ComboSpec{Delay: &DelaySpec{Time: 100}, Loss: &LossSpec{Percent: 20.0}})
	require.NoError(t, err)

	err = cmd.Run(context.Background(), true)
	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}

func TestComboCommand_Run_DryRun(t *testing.T) {
	tests := []struct {
		name     string
		spec     ComboSpec
		netemCmd []string
	}{
		{
			name: "delay with loss and corrupt",
			spec: ComboSpec{
				Delay:   &DelaySpec{Time: 100, Jitter: 10},
				Loss:    &LossSpec{Percent: 20.0},
				Corrupt: &CorruptSpec{Percent: 5.0},
			},
			netemCmd: []string{"delay", "100ms", "10ms", "loss", "20.00", "corrupt", "5.00"},
		},
		{
			name: "all effects fixed order",
			spec: ComboSpec{
				Delay:     &DelaySpec{Time: 200, Jitter: 50, Correlation: 25.5, Distribution: "normal"},
				Loss:      &LossSpec{Percent: 10.0, Correlation: 5.0},
				Duplicate: &DuplicateSpec{Percent: 3.0},
				Corrupt:   &CorruptSpec{Percent: 1.0},
				Rate:      &RateSpec{Rate: "100kbit"},
			},
			netemCmd: []string{
				"delay", "200ms", "50ms", "25.50", "distribution", "normal",
				"loss", "10.00", "5.00",
				"duplicate", "3.00",
				"corrupt", "1.00",
				"rate", "100kbit",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := container.NewMockClient(t)
			target := &container.Container{
				ContainerID:   "abc123",
				ContainerName: "target",
				Labels:        map[string]string{},
				Networks:      map[string]container.NetworkLink{},
			}
			gparams := &chaos.GlobalParams{Names: []string{"target"}, DryRun: true}
			nparams := &container.NetemRequest{
				Interface: "eth0",
				Duration:  100 * time.Millisecond,
				Sidecar:   container.SidecarSpec{Image: "tc"},
				DryRun:    true,
			}

			mockClient.EXPECT().ListContainers(mock.Anything,
				mock.AnythingOfType("container.FilterFunc"),
				container.ListOpts{All: false, Labels: nil}).
				Return([]*container.Container{target}, nil)

			expectedReq := &container.NetemRequest{
				Container: target,
				Interface: "eth0",
				Command:   tt.netemCmd,
				Duration:  100 * time.Millisecond,
				Sidecar:   container.SidecarSpec{Image: "tc"},
				DryRun:    true,
			}
			mockClient.EXPECT().NetemContainer(mock.Anything, expectedReq).Return(nil)
			mockClient.EXPECT().StopNetemContainer(mock.Anything, expectedReq).Return(nil)

			cmd, err := NewComboCommand(mockClient, gparams, nparams, 0, tt.spec)
			require.NoError(t, err)

			err = cmd.Run(context.Background(), false)
			assert.NoError(t, err)
			mockClient.AssertExpectations(t)
		})
	}
}
