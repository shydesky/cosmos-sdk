package manager

import (
	"fmt"

	"github.com/tendermint/tendermint/libs/log"

	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/params/types"
	"github.com/cosmos/cosmos-sdk/params/types/subspace"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// Manager of the global paramstore
type Manager struct {
	cdc    codec.Marshaler
	key    sdk.StoreKey
	tkey   sdk.StoreKey
	spaces map[string]*subspace.Subspace
}

// New constructs a params manager
func New(cdc codec.Marshaler, key, tkey sdk.StoreKey) Manager {
	return Manager{
		cdc:    cdc,
		key:    key,
		tkey:   tkey,
		spaces: make(map[string]*subspace.Subspace),
	}
}

// Logger returns a module-specific logger.
func (k Manager) Logger(ctx sdk.Context) log.Logger {
	return ctx.Logger().With("module", fmt.Sprintf("x/%s", types.ModuleName))
}

// Allocate subspace used for keepers
func (k Manager) Subspace(s string) subspace.Subspace {
	_, ok := k.spaces[s]
	if ok {
		panic("subspace already occupied")
	}

	if s == "" {
		panic("cannot use empty string for subspace")
	}

	space := subspace.NewSubspace(k.cdc, k.key, k.tkey, s)
	k.spaces[s] = &space

	return space
}

// Get existing substore from manager
func (k Manager) GetSubspace(s string) (subspace.Subspace, bool) {
	space, ok := k.spaces[s]
	if !ok {
		return subspace.Subspace{}, false
	}
	return *space, ok
}
