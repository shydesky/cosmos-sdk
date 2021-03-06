package keeper

// nolint:deadcode,unused
// DONTCOVER
// noalias

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/crypto"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"github.com/tendermint/tendermint/libs/log"
	dbm "github.com/tendermint/tm-db"

	"github.com/cosmos/cosmos-sdk/codec"
	simappcodec "github.com/cosmos/cosmos-sdk/simapp/codec"
	"github.com/cosmos/cosmos-sdk/store"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/bank"
	"github.com/cosmos/cosmos-sdk/x/params"
	"github.com/cosmos/cosmos-sdk/x/slashing/internal/types"
	"github.com/cosmos/cosmos-sdk/x/staking"
	"github.com/cosmos/cosmos-sdk/x/supply"
)

// TODO remove dependencies on staking (should only refer to validator set type from sdk)

var (
	Pks = []crypto.PubKey{
		newPubKey("0B485CFC0EECC619440448436F8FC9DF40566F2369E72400281454CB552AFB50"),
		newPubKey("0B485CFC0EECC619440448436F8FC9DF40566F2369E72400281454CB552AFB51"),
		newPubKey("0B485CFC0EECC619440448436F8FC9DF40566F2369E72400281454CB552AFB52"),
	}
	Addrs = []sdk.ValAddress{
		sdk.ValAddress(Pks[0].Address()),
		sdk.ValAddress(Pks[1].Address()),
		sdk.ValAddress(Pks[2].Address()),
	}
	InitTokens = sdk.TokensFromConsensusPower(200)
	initCoins  = sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, InitTokens))
)

func createTestCodec() *codec.Codec {
	cdc := codec.New()
	sdk.RegisterCodec(cdc)
	auth.RegisterCodec(cdc)
	supply.RegisterCodec(cdc)
	bank.RegisterCodec(cdc)
	staking.RegisterCodec(cdc)
	codec.RegisterCrypto(cdc)
	return cdc
}

func CreateTestInput(t *testing.T, defaults types.Params) (sdk.Context, bank.Keeper, staking.Keeper, params.Subspace, Keeper) {
	keyAcc := sdk.NewKVStoreKey(auth.StoreKey)
	keyBank := sdk.NewKVStoreKey(bank.StoreKey)
	keyStaking := sdk.NewKVStoreKey(staking.StoreKey)
	keySlashing := sdk.NewKVStoreKey(types.StoreKey)
	keySupply := sdk.NewKVStoreKey(supply.StoreKey)
	keyParams := sdk.NewKVStoreKey(params.StoreKey)
	tkeyParams := sdk.NewTransientStoreKey(params.TStoreKey)

	db := dbm.NewMemDB()

	ms := store.NewCommitMultiStore(db)
	ms.MountStoreWithDB(keyAcc, sdk.StoreTypeIAVL, db)
	ms.MountStoreWithDB(keyBank, sdk.StoreTypeIAVL, db)
	ms.MountStoreWithDB(keyStaking, sdk.StoreTypeIAVL, db)
	ms.MountStoreWithDB(keySupply, sdk.StoreTypeIAVL, db)
	ms.MountStoreWithDB(keySlashing, sdk.StoreTypeIAVL, db)
	ms.MountStoreWithDB(keyParams, sdk.StoreTypeIAVL, db)
	ms.MountStoreWithDB(tkeyParams, sdk.StoreTypeTransient, db)

	err := ms.LoadLatestVersion()
	require.Nil(t, err)

	ctx := sdk.NewContext(ms, abci.Header{Time: time.Unix(0, 0)}, false, log.NewNopLogger())
	cdc := createTestCodec()
	appCodec := simappcodec.NewAppCodec(cdc)

	feeCollectorAcc := supply.NewEmptyModuleAccount(auth.FeeCollectorName)
	notBondedPool := supply.NewEmptyModuleAccount(staking.NotBondedPoolName, supply.Burner, supply.Staking)
	bondPool := supply.NewEmptyModuleAccount(staking.BondedPoolName, supply.Burner, supply.Staking)

	blacklistedAddrs := make(map[string]bool)
	blacklistedAddrs[feeCollectorAcc.GetAddress().String()] = true
	blacklistedAddrs[notBondedPool.GetAddress().String()] = true
	blacklistedAddrs[bondPool.GetAddress().String()] = true

	paramsKeeper := params.NewKeeper(params.ModuleCdc, keyParams, tkeyParams)
	accountKeeper := auth.NewAccountKeeper(appCodec, keyAcc, paramsKeeper.Subspace(auth.DefaultParamspace), auth.ProtoBaseAccount)

	bk := bank.NewBaseKeeper(cdc, keyBank, accountKeeper, paramsKeeper.Subspace(bank.DefaultParamspace), blacklistedAddrs)
	maccPerms := map[string][]string{
		auth.FeeCollectorName:     nil,
		staking.NotBondedPoolName: {supply.Burner, supply.Staking},
		staking.BondedPoolName:    {supply.Burner, supply.Staking},
	}
	supplyKeeper := supply.NewKeeper(appCodec, keySupply, accountKeeper, bk, maccPerms)

	totalSupply := sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, InitTokens.MulRaw(int64(len(Addrs)))))
	supplyKeeper.SetSupply(ctx, supply.NewSupply(totalSupply))

	sk := staking.NewKeeper(staking.ModuleCdc, keyStaking, bk, supplyKeeper, paramsKeeper.Subspace(staking.DefaultParamspace))
	genesis := staking.DefaultGenesisState()

	// set module accounts
	supplyKeeper.SetModuleAccount(ctx, feeCollectorAcc)
	supplyKeeper.SetModuleAccount(ctx, bondPool)
	supplyKeeper.SetModuleAccount(ctx, notBondedPool)

	_ = staking.InitGenesis(ctx, sk, accountKeeper, bk, supplyKeeper, genesis)

	for i, addr := range Addrs {
		addr := sdk.AccAddress(addr)
		accountKeeper.SetAccount(ctx, auth.NewBaseAccount(addr, Pks[i], uint64(i), 0))
		require.NoError(t, bk.SetBalances(ctx, addr, initCoins))
	}

	paramstore := paramsKeeper.Subspace(types.DefaultParamspace)
	keeper := NewKeeper(cdc, keySlashing, &sk, paramstore)

	keeper.SetParams(ctx, defaults)
	sk.SetHooks(keeper.Hooks())

	return ctx, bk, sk, paramstore, keeper
}

func newPubKey(pk string) (res crypto.PubKey) {
	pkBytes, err := hex.DecodeString(pk)
	if err != nil {
		panic(err)
	}
	var pkEd ed25519.PubKeyEd25519
	copy(pkEd[:], pkBytes)
	return pkEd
}

// Have to change these parameters for tests
// lest the tests take forever
func TestParams() types.Params {
	params := types.DefaultParams()
	params.SignedBlocksWindow = 1000
	params.DowntimeJailDuration = 60 * 60
	return params
}

func NewTestMsgCreateValidator(address sdk.ValAddress, pubKey crypto.PubKey, amt sdk.Int) staking.MsgCreateValidator {
	commission := staking.NewCommissionRates(sdk.ZeroDec(), sdk.ZeroDec(), sdk.ZeroDec())
	return staking.NewMsgCreateValidator(
		address, pubKey, sdk.NewCoin(sdk.DefaultBondDenom, amt),
		staking.Description{}, commission, sdk.OneInt(),
	)
}

func NewTestMsgDelegate(delAddr sdk.AccAddress, valAddr sdk.ValAddress, delAmount sdk.Int) staking.MsgDelegate {
	amount := sdk.NewCoin(sdk.DefaultBondDenom, delAmount)
	return staking.NewMsgDelegate(delAddr, valAddr, amount)
}
