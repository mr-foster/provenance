package simulation_test

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"

	simappparams "github.com/cosmos/cosmos-sdk/simapp/params"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	simapp "github.com/provenance-io/provenance/app"
	"github.com/provenance-io/provenance/x/name/keeper"
	"github.com/provenance-io/provenance/x/name/simulation"
	"github.com/provenance-io/provenance/x/name/types"
)

func TestProposalContents(t *testing.T) {
	app := simapp.Setup(false)
	ctx := app.BaseApp.NewContext(false, tmproto.Header{})

	// initialize parameters
	s := rand.NewSource(1)
	r := rand.New(s)

	accounts := simtypes.RandomAccounts(r, 3)

	// execute ProposalContents function
	// TODO replace with simapp keeper instance for name module
	weightedProposalContent := simulation.ProposalContents(keeper.NewKeeper(app.AppCodec(), app.GetKey(types.ModuleName), app.GetSubspace(types.ModuleName), app.AccountKeeper))
	require.Len(t, weightedProposalContent, 1)

	w0 := weightedProposalContent[0]

	// tests w0 interface:
	require.Equal(t, simulation.OpWeightSubmitCreateRootNameProposal, w0.AppParamsKey())
	require.Equal(t, simappparams.DefaultWeightTextProposal, w0.DefaultWeight())

	content := w0.ContentSimulatorFn()(r, ctx, accounts)

	require.Equal(t, "hPjMaxKlMIJMOXcnQfyzeOcbWwNbeHVIkPZBSpYuLyYggwexjxusrBqDOTtGTOWeLrQKjLxzIivHSlcxgdXhhuTSkuxKGLwQvuyN", content.GetDescription())
	require.Equal(t, "eAerqyNEUz", content.GetTitle())
	require.Equal(t, "name", content.ProposalRoute())
	require.Equal(t, "CreateRootNameProposal", content.ProposalType())
}
