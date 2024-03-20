package asterisc

import (
	"context"
	"fmt"

	"github.com/ethereum-optimism/asterisc/rvgo/fast"
	"github.com/ethereum-optimism/optimism/op-challenger/game/fault/types"
	"github.com/ethereum/go-ethereum/common"
)

var _ types.PrestateProvider = (*AsteriscPreStateProvider)(nil)

type AsteriscPreStateProvider struct {
	prestate string

	prestateCommitment common.Hash
}

func NewPrestateProvider(prestate string) *AsteriscPreStateProvider {
	return &AsteriscPreStateProvider{prestate: prestate}
}

func (p *AsteriscPreStateProvider) absolutePreState() ([]byte, error) {
	state, err := parseState(p.prestate)
	if err != nil {
		return nil, fmt.Errorf("cannot load absolute pre-state: %w", err)
	}
	return state.EncodeWitness(), nil
}

func (p *AsteriscPreStateProvider) AbsolutePreStateCommitment(_ context.Context) (common.Hash, error) {
	if p.prestateCommitment != (common.Hash{}) {
		return p.prestateCommitment, nil
	}
	state, err := p.absolutePreState()
	if err != nil {
		return common.Hash{}, fmt.Errorf("cannot load absolute pre-state: %w", err)
	}
	hash, err := fast.StateWitness(state).StateHash()
	if err != nil {
		return common.Hash{}, fmt.Errorf("cannot hash absolute pre-state: %w", err)
	}
	p.prestateCommitment = hash
	return hash, nil
}
