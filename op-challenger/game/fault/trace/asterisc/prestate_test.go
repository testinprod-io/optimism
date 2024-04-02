package asterisc

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ethereum-optimism/asterisc/rvgo/fast"
	"github.com/stretchr/testify/require"
)

func newAsteriscPrestateProvider(dataDir string, prestate string) *AsteriscPreStateProvider {
	return &AsteriscPreStateProvider{
		prestate: filepath.Join(dataDir, prestate),
	}
}

func TestAbsolutePreStateCommitment(t *testing.T) {
	dataDir := t.TempDir()

	prestate := "state.json"

	t.Run("StateUnavailable", func(t *testing.T) {
		provider := newAsteriscPrestateProvider("/dir/does/not/exist", prestate)
		_, err := provider.AbsolutePreStateCommitment(context.Background())
		require.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("InvalidStateFile", func(t *testing.T) {
		setupPreState(t, dataDir, "invalid.json")
		provider := newAsteriscPrestateProvider(dataDir, prestate)
		_, err := provider.AbsolutePreStateCommitment(context.Background())
		require.ErrorContains(t, err, "invalid asterisc state")
	})

	t.Run("ExpectedAbsolutePreState", func(t *testing.T) {
		setupPreState(t, dataDir, "state.json")
		provider := newAsteriscPrestateProvider(dataDir, prestate)
		actual, err := provider.AbsolutePreStateCommitment(context.Background())
		require.NoError(t, err)
		// TODO(pcw109550) fill this in
		state := fast.VMState{}
		// for cannon(must be removed because this is asterisc);
		// state := mipsevm.State{
		// 	Memory:         mipsevm.NewMemory(),
		// 	PreimageKey:    common.HexToHash("cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"),
		// 	PreimageOffset: 0,
		// 	PC:             0,
		// 	NextPC:         1,
		// 	LO:             0,
		// 	HI:             0,
		// 	Heap:           0,
		// 	ExitCode:       0,
		// 	Exited:         false,
		// 	Step:           0,
		// 	Registers:      [32]uint32{},
		// }
		expected, err := state.EncodeWitness().StateHash()
		require.NoError(t, err)
		require.Equal(t, expected, actual)
	})

	t.Run("CacheAbsolutePreState", func(t *testing.T) {
		setupPreState(t, dataDir, "state.json")
		provider := newAsteriscPrestateProvider(dataDir, prestate)
		first, err := provider.AbsolutePreStateCommitment(context.Background())
		require.NoError(t, err)

		// Remove the prestate from disk
		require.NoError(t, os.Remove(provider.prestate))

		// Value should still be available from cache
		cached, err := provider.AbsolutePreStateCommitment(context.Background())
		require.NoError(t, err)
		require.Equal(t, first, cached)
	})
}

func setupPreState(t *testing.T, dataDir string, filename string) {
	srcDir := filepath.Join("test_data")
	path := filepath.Join(srcDir, filename)
	file, err := testData.ReadFile(path)
	require.NoErrorf(t, err, "reading %v", path)
	err = os.WriteFile(filepath.Join(dataDir, "state.json"), file, 0o644)
	require.NoErrorf(t, err, "writing %v", path)
}
