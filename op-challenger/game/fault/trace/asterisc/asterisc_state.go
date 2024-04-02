package asterisc

import (
	"encoding/json"
	"fmt"

	asterisc "github.com/ethereum-optimism/asterisc/rvgo/fast"
	"github.com/ethereum-optimism/optimism/op-service/ioutil"
)

func parseState(path string) (*asterisc.VMState, error) {
	file, err := ioutil.OpenDecompressed(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open state file (%v): %w", path, err)
	}
	defer file.Close()
	var state asterisc.VMState
	err = json.NewDecoder(file).Decode(&state)
	if err != nil {
		return nil, fmt.Errorf("invalid asterisc VM state (%v): %w", path, err)
	}
	return &state, nil
}
