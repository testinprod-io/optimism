package asterisc

import (
	"encoding/json"
	"fmt"

	"github.com/ethereum-optimism/asterisc/rvgo/fast"
	"github.com/ethereum-optimism/optimism/op-service/ioutil"
)

func parseState(path string) (*fast.VMState, error) {
	file, err := ioutil.OpenDecompressed(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open state file (%v): %w", path, err)
	}
	defer file.Close()
	var state fast.VMState
	err = json.NewDecoder(file).Decode(&state)
	if err != nil {
		return nil, fmt.Errorf("invalid asterisc VM state (%v): %w", path, err)
	}
	return &state, nil
}
