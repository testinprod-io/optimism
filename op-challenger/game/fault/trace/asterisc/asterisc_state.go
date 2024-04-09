package asterisc

import (
	"encoding/json"
	"fmt"

	"github.com/ethereum-optimism/optimism/op-service/ioutil"
)

// The state struct will be read from json.
// other fields included in json are specific to FPVM implementation, and not required for tracer
type VMState struct {
	PC        uint64   `json:"pc"`
	Exited    bool     `json:"exited"`
	Step      uint64   `json:"step"`
	Witness   []byte   `json:"witness"`
	StateHash [32]byte `json:"stateHash"`
}

func (state *VMState) validateStateHash() error {
	highOrderByte := state.StateHash[0]
	if highOrderByte >= 4 { // must be enum below 4
		return fmt.Errorf("invalid stateHash: high order byte: %d", highOrderByte)
	}
	if !state.Exited && highOrderByte != 3 { // VMStatusUnfinished
		return fmt.Errorf("invalid stateHash: high order byte must be 3 but got %d", highOrderByte)
	}
	return nil
}

func (state *VMState) validateWitness() error {
	witnessLen := len(state.Witness)
	if witnessLen != 362 {
		return fmt.Errorf("invalid witness: Length must be 362 but got %d", witnessLen)
	}
	return nil
}

func (state *VMState) validateState() error {
	if err := state.validateStateHash(); err != nil {
		return err
	}
	if err := state.validateWitness(); err != nil {
		return err
	}
	return nil
}

func parseState(path string) (*VMState, error) {
	file, err := ioutil.OpenDecompressed(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open state file (%v): %w", path, err)
	}
	defer file.Close()
	var state VMState
	if err := json.NewDecoder(file).Decode(&state); err != nil {
		return nil, fmt.Errorf("invalid asterisc VM state (%v): %w", path, err)
	}
	if err := state.validateState(); err != nil {
		return nil, fmt.Errorf("invalid asterisc VM state (%v): %w", path, err)
	}
	return &state, nil
}
