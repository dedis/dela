package pedersen

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDKGState(t *testing.T) {
	states := []dkgState{
		initial,
		sharing,
		certified,
		resharing,
		0xaa,
	}
	strings := []string{
		"Initial",
		"Sharing",
		"Certified",
		"Resharing",
		"UNKNOWN",
	}

	for i := range states {
		require.Equal(t, strings[i], states[i].String())
	}
}

func TestSwitchState(t *testing.T) {
	state := state{dkgState: initial}

	err := state.switchState(initial)
	require.EqualError(t, err, "initial state cannot be set manually")

	state.dkgState = certified

	err = state.switchState(sharing)
	require.EqualError(t, err, "sharing state must switch from initial: Certified")

	state.dkgState = initial

	err = state.switchState(certified)
	require.EqualError(t, err, "certified state must switch from sharing or resharing: Initial")

	state.dkgState = resharing

	err = state.switchState(resharing)
	require.EqualError(t, err, "resharing state must switch from initial or certified: Resharing")
}

func TestCheckStateUnknown(t *testing.T) {
	state := state{}

	err := state.checkState(0xaa)
	require.EqualError(t, err, "unexpected state: Initial != one of [UNKNOWN]")
}
