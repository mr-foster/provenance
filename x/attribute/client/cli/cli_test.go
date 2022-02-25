package cli_test

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	tmcli "github.com/tendermint/tendermint/libs/cli"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	clitestutil "github.com/cosmos/cosmos-sdk/testutil/cli"
	testnet "github.com/cosmos/cosmos-sdk/testutil/network"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx"
	authcli "github.com/cosmos/cosmos-sdk/x/auth/client/cli"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"

	"github.com/provenance-io/provenance/testutil"
	"github.com/provenance-io/provenance/x/attribute/client/cli"
	attributetypes "github.com/provenance-io/provenance/x/attribute/types"
	mdtypes "github.com/provenance-io/provenance/x/metadata/types"
	namecli "github.com/provenance-io/provenance/x/name/client/cli"
	nametypes "github.com/provenance-io/provenance/x/name/types"
)

type IntegrationTestSuite struct {
	suite.Suite

	cfg     testnet.Config
	testnet *testnet.Network

	keyring    keyring.Keyring
	keyringDir string
	accounts   []keyring.Info

	account1Addr sdk.AccAddress
	account1Str  string

	account2Addr sdk.AccAddress
	account2Str  string

	account3Addr sdk.AccAddress
	account3Str  string

	account4Addr sdk.AccAddress
	account4Str  string

	account5Addr sdk.AccAddress
	account5Str  string

	accAttrCount int
}

func TestIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(IntegrationTestSuite))
}

func (s *IntegrationTestSuite) SetupSuite() {
	var err error
	s.GenerateAccountsWithKeyrings(5)

	s.account1Addr = s.accounts[0].GetAddress()
	s.account1Str = s.account1Addr.String()

	s.account2Addr = s.accounts[1].GetAddress()
	s.account2Str = s.account2Addr.String()

	s.account3Addr = s.accounts[2].GetAddress()
	s.account3Str = s.account3Addr.String()

	s.account4Addr = s.accounts[3].GetAddress()
	s.account4Str = s.account4Addr.String()

	s.account5Addr = s.accounts[4].GetAddress()
	s.account5Str = s.account5Addr.String()

	s.accAttrCount = 500

	s.T().Log("setting up integration test suite")

	cfg := testutil.DefaultTestNetworkConfig()

	genesisState := cfg.GenesisState
	cfg.NumValidators = 1

	// Configure Genesis data for name module
	var nameData nametypes.GenesisState
	nameData.Bindings = append(nameData.Bindings, nametypes.NewNameRecord("attribute", s.account1Addr, false))
	nameData.Bindings = append(nameData.Bindings, nametypes.NewNameRecord("example.attribute", s.account1Addr, false))
	nameData.Bindings = append(nameData.Bindings, nametypes.NewNameRecord("pb", s.account5Addr, false))
	nameData.Bindings = append(nameData.Bindings, nametypes.NewNameRecord("pb1", s.account1Addr, false))
	nameData.Params.AllowUnrestrictedNames = false
	nameData.Params.MaxNameLevels = 3
	nameData.Params.MinSegmentLength = 2
	nameData.Params.MaxSegmentLength = 12
	nameDataBz, err := cfg.Codec.MarshalJSON(&nameData)
	s.Require().NoError(err)
	genesisState[nametypes.ModuleName] = nameDataBz

	var authData authtypes.GenesisState
	s.Require().NoError(cfg.Codec.UnmarshalJSON(genesisState[authtypes.ModuleName], &authData))
	genAccount1, err := codectypes.NewAnyWithValue(&authtypes.BaseAccount{
		Address:       s.account1Str,
		AccountNumber: 1,
		Sequence:      0,
	})
	s.Require().NoError(err)
	genAccount5, err := codectypes.NewAnyWithValue(&authtypes.BaseAccount{
		Address:       s.account5Str,
		AccountNumber: 1,
		Sequence:      0,
	})
	s.Require().NoError(err)
	authData.Accounts = append(authData.Accounts, genAccount1)
	authData.Accounts = append(authData.Accounts, genAccount5)
	authDataBz, err := cfg.Codec.MarshalJSON(&authData)
	s.Require().NoError(err)
	genesisState[authtypes.ModuleName] = authDataBz

	balances := sdk.NewCoins(
		sdk.NewCoin(cfg.BondDenom, cfg.AccountTokens),
	)
	var bankData banktypes.GenesisState
	s.Require().NoError(cfg.Codec.UnmarshalJSON(genesisState[banktypes.ModuleName], &bankData))
	genBank1 := banktypes.Balance{Address: s.account1Str, Coins: balances.Sort()}
	genBank5 := banktypes.Balance{Address: s.account5Str, Coins: balances.Sort()}
	s.Require().NoError(err)
	bankData.Balances = append(bankData.Balances, genBank1)
	bankData.Balances = append(bankData.Balances, genBank5)
	bankDataBz, err := cfg.Codec.MarshalJSON(&bankData)
	s.Require().NoError(err)
	genesisState[banktypes.ModuleName] = bankDataBz

	// Configure Genesis data for attribute module
	var attributeData attributetypes.GenesisState
	attributeData.Attributes = append(attributeData.Attributes,
		attributetypes.NewAttribute(
			"example.attribute",
			s.account1Str,
			attributetypes.AttributeType_String,
			[]byte("example attribute value string")))
	attributeData.Attributes = append(attributeData.Attributes,
		attributetypes.NewAttribute(
			"example.attribute.count",
			s.account1Str,
			attributetypes.AttributeType_Int,
			[]byte("2")))
	for i := 0; i < s.accAttrCount; i++ {
		attributeData.Attributes = append(attributeData.Attributes,
			attributetypes.NewAttribute(
				fmt.Sprintf("example.attribute.%s", toWritten(i)),
				s.account3Str,
				attributetypes.AttributeType_Int,
				[]byte(fmt.Sprintf("%d", i))))
		attributeData.Attributes = append(attributeData.Attributes,
			attributetypes.NewAttribute(
				"example.attribute.overload",
				s.account4Str,
				attributetypes.AttributeType_String,
				[]byte(toWritten(i))))
	}
	attributeData.Params.MaxValueLength = 128
	attributeDataBz, err := cfg.Codec.MarshalJSON(&attributeData)
	s.Require().NoError(err)
	genesisState[attributetypes.ModuleName] = attributeDataBz

	cfg.GenesisState = genesisState

	s.cfg = cfg

	s.testnet = testnet.New(s.T(), cfg)

	_, err = s.testnet.WaitForHeight(1)
	s.Require().NoError(err)
}

func (s *IntegrationTestSuite) TearDownSuite() {
	s.testnet.WaitForNextBlock()
	s.T().Log("tearing down integration test suite")
	s.testnet.Cleanup()
}

func (s *IntegrationTestSuite) GenerateAccountsWithKeyrings(number int) {
	path := hd.CreateHDPath(118, 0, 0).String()
	s.keyringDir = s.T().TempDir()
	kr, err := keyring.New(s.T().Name(), "test", s.keyringDir, nil)
	s.Require().NoError(err)
	s.keyring = kr
	for i := 0; i < number; i++ {
		keyId := fmt.Sprintf("test_key%v", i)
		info, _, err := kr.NewMnemonic(keyId, keyring.English, path, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
		s.Require().NoError(err)
		s.accounts = append(s.accounts, info)
	}
}

// toWritten converts an integer to a written string version.
// Originally, this was the full written string, e.g. 38 => "thirtyEight" but that ended up being too long for
// an attribute name segment, so it got trimmed down, e.g. 115 => "onehun15".
func toWritten(i int) string {
	if i < 0 || i > 999 {
		panic("cannot convert negative numbers or numbers larger than 999 to written string")
	}
	switch i {
	case 0:
		return "zero"
	case 1:
		return "one"
	case 2:
		return "two"
	case 3:
		return "three"
	case 4:
		return "four"
	case 5:
		return "five"
	case 6:
		return "six"
	case 7:
		return "seven"
	case 8:
		return "eight"
	case 9:
		return "nine"
	case 10:
		return "ten"
	case 11:
		return "eleven"
	case 12:
		return "twelve"
	case 13:
		return "thirteen"
	case 14:
		return "fourteen"
	case 15:
		return "fifteen"
	case 16:
		return "sixteen"
	case 17:
		return "seventeen"
	case 18:
		return "eighteen"
	case 19:
		return "nineteen"
	case 20:
		return "twenty"
	case 30:
		return "thirty"
	case 40:
		return "forty"
	case 50:
		return "fifty"
	case 60:
		return "sixty"
	case 70:
		return "seventy"
	case 80:
		return "eighty"
	case 90:
		return "ninety"
	default:
		var r int
		var l string
		switch {
		case i < 100:
			r = i % 10
			l = toWritten(i - r)
		default:
			r = i % 100
			l = toWritten(i/100) + "hun"
		}
		if r == 0 {
			return l
		}
		return l + fmt.Sprintf("%d", r)
	}
}

func limitArg(pageSize int) string {
	return fmt.Sprintf("--limit=%d", pageSize)
}

func pageKeyArg(nextKey string) string {
	return fmt.Sprintf("--page-key=%s", nextKey)
}

// attrSorter implements sort.Interface for []Attribute
// Sorts by .Name then .AttributeType then .Value, then .Address.
type attrSorter []attributetypes.Attribute

func (a attrSorter) Len() int {
	return len(a)
}
func (a attrSorter) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}
func (a attrSorter) Less(i, j int) bool {
	// Sort by Name first
	if a[i].Name != a[j].Name {
		return a[i].Name < a[j].Name
	}
	// Then by AttributeType
	if a[i].AttributeType != a[j].AttributeType {
		return a[i].AttributeType < a[j].AttributeType
	}
	// Then by Value.
	// Since this is unit tests, just use the raw byte values rather than going through the trouble of using the AttributeType and converting them.
	for _, vbi := range a[i].Value {
		for _, vbj := range a[j].Value {
			if vbi != vbj {
				return vbi < vbj
			}
		}
	}
	// Then by Address.
	return a[i].Address < a[j].Address
}

func (s *IntegrationTestSuite) TestGetAccountAttributeCmd() {
	testCases := []struct {
		name           string
		args           []string
		expectedOutput string
	}{
		{
			"should get attribute by name with json output",
			[]string{s.account1Addr.String(), "example.attribute", fmt.Sprintf("--%s=json", tmcli.OutputFlag)},
			fmt.Sprintf(`{"account":"%s","attributes":[{"name":"example.attribute","value":"ZXhhbXBsZSBhdHRyaWJ1dGUgdmFsdWUgc3RyaW5n","attribute_type":"ATTRIBUTE_TYPE_STRING","address":"%s"}],"pagination":{"next_key":null,"total":"0"}}`, s.account1Addr.String(), s.account1Addr.String()),
		},
		{
			"should get attribute by name with text output",
			[]string{s.account1Addr.String(), "example.attribute", fmt.Sprintf("--%s=text", tmcli.OutputFlag)},
			fmt.Sprintf(`account: %s
attributes:
- address: %s
  attribute_type: ATTRIBUTE_TYPE_STRING
  name: example.attribute
  value: ZXhhbXBsZSBhdHRyaWJ1dGUgdmFsdWUgc3RyaW5n
pagination:
  next_key: null
  total: "0"`, s.account1Addr.String(), s.account1Addr.String()),
		},
		{
			"should fail to find unknown attribute output",
			[]string{s.account1Addr.String(), "example.none", fmt.Sprintf("--%s=text", tmcli.OutputFlag)},
			fmt.Sprintf(`account: %s
attributes: []
pagination:
  next_key: null
  total: "0"`, s.account1Addr.String()),
		},
		{
			"should fail to find unknown attribute by name with json output",
			[]string{s.account1Addr.String(), "example.none", fmt.Sprintf("--%s=json", tmcli.OutputFlag)},
			fmt.Sprintf(`{"account":"%s","attributes":[],"pagination":{"next_key":null,"total":"0"}}`, s.account1Addr.String()),
		},
	}

	for _, tc := range testCases {
		tc := tc

		s.Run(tc.name, func() {
			cmd := cli.GetAccountAttributeCmd()
			clientCtx := s.testnet.Validators[0].ClientCtx

			out, err := clitestutil.ExecTestCLICmd(clientCtx, cmd, tc.args)
			s.Require().NoError(err)
			s.Require().Equal(tc.expectedOutput, strings.TrimSpace(out.String()))
		})
	}
}

func (s *IntegrationTestSuite) TestScanAccountAttributesCmd() {
	testCases := []struct {
		name           string
		args           []string
		expectedOutput string
	}{
		{
			"should get attribute by suffix with json output",
			[]string{s.account1Addr.String(), "attribute", fmt.Sprintf("--%s=json", tmcli.OutputFlag)},
			fmt.Sprintf(`{"account":"%s","attributes":[{"name":"example.attribute","value":"ZXhhbXBsZSBhdHRyaWJ1dGUgdmFsdWUgc3RyaW5n","attribute_type":"ATTRIBUTE_TYPE_STRING","address":"%s"}],"pagination":{"next_key":null,"total":"0"}}`, s.account1Addr.String(), s.account1Addr.String()),
		},
		{
			"should get attribute by suffix with text output",
			[]string{s.account1Addr.String(), "attribute", fmt.Sprintf("--%s=text", tmcli.OutputFlag)},
			fmt.Sprintf(`account: %s
attributes:
- address: %s
  attribute_type: ATTRIBUTE_TYPE_STRING
  name: example.attribute
  value: ZXhhbXBsZSBhdHRyaWJ1dGUgdmFsdWUgc3RyaW5n
pagination:
  next_key: null
  total: "0"`, s.account1Addr.String(), s.account1Addr.String()),
		},
		{
			"should fail to find unknown attribute suffix text output",
			[]string{s.account1Addr.String(), "none", fmt.Sprintf("--%s=text", tmcli.OutputFlag)},
			fmt.Sprintf(`account: %s
attributes: []
pagination:
  next_key: null
  total: "0"`, s.account1Addr.String()),
		},
		{
			"should get attribute by suffix with json output",
			[]string{s.account1Addr.String(), "none", fmt.Sprintf("--%s=json", tmcli.OutputFlag)},
			fmt.Sprintf(`{"account":"%s","attributes":[],"pagination":{"next_key":null,"total":"0"}}`, s.account1Addr.String()),
		},
	}

	for _, tc := range testCases {
		tc := tc

		s.Run(tc.name, func() {
			cmd := cli.ScanAccountAttributesCmd()
			clientCtx := s.testnet.Validators[0].ClientCtx

			out, err := clitestutil.ExecTestCLICmd(clientCtx, cmd, tc.args)
			s.Require().NoError(err)
			s.Require().Equal(tc.expectedOutput, strings.TrimSpace(out.String()))
		})
	}
}

func (s *IntegrationTestSuite) TestListAccountAttributesCmd() {
	testCases := []struct {
		name           string
		args           []string
		expectedOutput string
	}{
		{
			"should list all attributes for account with json output",
			[]string{s.account1Addr.String(), fmt.Sprintf("--%s=json", tmcli.OutputFlag)},
			fmt.Sprintf(`{"account":"%s","attributes":[{"name":"example.attribute.count","value":"Mg==","attribute_type":"ATTRIBUTE_TYPE_INT","address":"%s"},{"name":"example.attribute","value":"ZXhhbXBsZSBhdHRyaWJ1dGUgdmFsdWUgc3RyaW5n","attribute_type":"ATTRIBUTE_TYPE_STRING","address":"%s"}],"pagination":{"next_key":null,"total":"0"}}`, s.account1Addr.String(), s.account1Addr.String(), s.account1Addr.String()),
		},
		{
			"should list all attributes for account text output",
			[]string{s.account1Addr.String(), fmt.Sprintf("--%s=text", tmcli.OutputFlag)},
			fmt.Sprintf(`account: %s
attributes:
- address: %s
  attribute_type: ATTRIBUTE_TYPE_INT
  name: example.attribute.count
  value: Mg==
- address: %s
  attribute_type: ATTRIBUTE_TYPE_STRING
  name: example.attribute
  value: ZXhhbXBsZSBhdHRyaWJ1dGUgdmFsdWUgc3RyaW5n
pagination:
  next_key: null
  total: "0"`, s.account1Addr.String(), s.account1Addr.String(), s.account1Addr.String()),
		},
	}

	for _, tc := range testCases {
		tc := tc

		s.Run(tc.name, func() {
			cmd := cli.ListAccountAttributesCmd()
			clientCtx := s.testnet.Validators[0].ClientCtx

			out, err := clitestutil.ExecTestCLICmd(clientCtx, cmd, tc.args)
			s.Require().NoError(err)
			s.Require().Equal(tc.expectedOutput, strings.TrimSpace(out.String()))
		})
	}
}

func (s *IntegrationTestSuite) TestGetAttributeParamsCmd() {
	testCases := []struct {
		name           string
		args           []string
		expectedOutput string
	}{
		{
			"json output",
			[]string{fmt.Sprintf("--%s=json", tmcli.OutputFlag)},
			"{\"max_value_length\":128}",
		},
		{
			"text output",
			[]string{fmt.Sprintf("--%s=text", tmcli.OutputFlag)},
			"max_value_length: 128",
		},
	}

	for _, tc := range testCases {
		tc := tc

		s.Run(tc.name, func() {
			cmd := cli.GetAttributeParamsCmd()
			clientCtx := s.testnet.Validators[0].ClientCtx

			out, err := clitestutil.ExecTestCLICmd(clientCtx, cmd, tc.args)
			s.Require().NoError(err)
			s.Require().Equal(tc.expectedOutput, strings.TrimSpace(out.String()))
		})
	}
}

func (s *IntegrationTestSuite) TestAttributeTxCommands() {

	testCases := []struct {
		name         string
		cmd          *cobra.Command
		args         []string
		expectErr    bool
		respType     proto.Message
		expectedCode uint32
	}{
		{
			"bind a new attribute name for testing",
			namecli.GetBindNameCmd(),
			[]string{
				"txtest",
				s.testnet.Validators[0].Address.String(),
				"attribute",
				fmt.Sprintf("--%s=%s", flags.FlagFrom, s.testnet.Validators[0].Address.String()),
				fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
				fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
				fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
			},
			false, &sdk.TxResponse{}, 0,
		},
		{
			"set attribute, valid string",
			cli.NewAddAccountAttributeCmd(),
			[]string{
				"txtest.attribute",
				s.testnet.Validators[0].Address.String(),
				"string",
				"test value",
				fmt.Sprintf("--%s=%s", flags.FlagFrom, s.testnet.Validators[0].Address.String()),
				fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
				fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
				fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
			},
			false, &sdk.TxResponse{}, 0,
		},
		{
			"set attribute, invalid bech32 address",
			cli.NewAddAccountAttributeCmd(),
			[]string{
				"txtest.attribute",
				"invalidbech32",
				"string",
				"test value",
				fmt.Sprintf("--%s=%s", flags.FlagFrom, s.testnet.Validators[0].Address.String()),
				fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
				fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
				fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
			},
			true, &sdk.TxResponse{}, 0,
		},
		{
			"set attribute, invalid type",
			cli.NewAddAccountAttributeCmd(),
			[]string{
				"txtest.attribute",
				s.account2Addr.String(),
				"blah",
				"3.14159",
				fmt.Sprintf("--%s=%s", flags.FlagFrom, s.testnet.Validators[0].Address.String()),
				fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
				fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
				fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
			},
			true, &sdk.TxResponse{}, 1,
		},
		{
			"set attribute, cannot encode",
			cli.NewAddAccountAttributeCmd(),
			[]string{
				"txtest.attribute",
				s.testnet.Validators[0].Address.String(),
				"bytes",
				"3.14159",
				fmt.Sprintf("--%s=%s", flags.FlagFrom, s.testnet.Validators[0].Address.String()),
				fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
				fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
				fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
			},
			true, &sdk.TxResponse{}, 1,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			clientCtx := s.testnet.Validators[0].ClientCtx
			out, err := clitestutil.ExecTestCLICmd(clientCtx, tc.cmd, tc.args)

			if tc.expectErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
				s.Require().NoError(clientCtx.JSONCodec.UnmarshalJSON(out.Bytes(), tc.respType), out.String())
				txResp := tc.respType.(*sdk.TxResponse)
				s.Require().Equal(tc.expectedCode, txResp.Code)
			}
		})
	}
}

func (s *IntegrationTestSuite) TestUpdateAccountAttributeTxCommands() {

	testCases := []struct {
		name         string
		cmd          *cobra.Command
		args         []string
		expectErr    bool
		respType     proto.Message
		expectedCode uint32
	}{
		{
			"bind a new attribute name for delete testing",
			namecli.GetBindNameCmd(),
			[]string{
				"updatetest",
				s.testnet.Validators[0].Address.String(),
				"attribute",
				fmt.Sprintf("--%s=%s", flags.FlagFrom, s.testnet.Validators[0].Address.String()),
				fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
				fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
				fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
			},
			false, &sdk.TxResponse{}, 0,
		},
		{
			"add new attribute for updating",
			cli.NewAddAccountAttributeCmd(),
			[]string{
				"updatetest.attribute",
				s.account2Addr.String(),
				"string",
				"test value",
				fmt.Sprintf("--%s=%s", flags.FlagFrom, s.testnet.Validators[0].Address.String()),
				fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
				fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
				fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
			},
			false, &sdk.TxResponse{}, 0,
		},
		{
			"fail to update attribute, account address failure",
			cli.NewUpdateAccountAttributeCmd(),
			[]string{
				"updatetest.attribute",
				"not-an-address",
				"string",
				"test value",
				"int",
				"10",
				fmt.Sprintf("--%s=%s", flags.FlagFrom, s.testnet.Validators[0].Address.String()),
				fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
				fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
				fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
			},
			true, &sdk.TxResponse{}, 0,
		},
		{
			"fail to update attribute, incorrect original type",
			cli.NewUpdateAccountAttributeCmd(),
			[]string{
				"updatetest.attribute",
				s.account2Addr.String(),
				"invalid",
				"test value",
				"int",
				"10",
				fmt.Sprintf("--%s=%s", flags.FlagFrom, s.testnet.Validators[0].Address.String()),
				fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
				fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
				fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
			},
			true, &sdk.TxResponse{}, 0,
		},
		{
			"fail to update attribute, incorrect update type",
			cli.NewUpdateAccountAttributeCmd(),
			[]string{
				"updatetest.attribute",
				s.account2Addr.String(),
				"string",
				"test value",
				"invalid",
				"10",
				fmt.Sprintf("--%s=%s", flags.FlagFrom, s.testnet.Validators[0].Address.String()),
				fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
				fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
				fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
			},
			true, &sdk.TxResponse{}, 0,
		},
		{
			"fail to update attribute, validate basic fail",
			cli.NewUpdateAccountAttributeCmd(),
			[]string{
				"updatetest.attribute",
				s.account2Addr.String(),
				"string",
				"test value",
				"init",
				"nan",
				fmt.Sprintf("--%s=%s", flags.FlagFrom, s.testnet.Validators[0].Address.String()),
				fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
				fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
				fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
			},
			true, &sdk.TxResponse{}, 0,
		},
		{
			"successful update of attribute",
			cli.NewUpdateAccountAttributeCmd(),
			[]string{
				"updatetest.attribute",
				s.account2Addr.String(),
				"string",
				"test value",
				"int",
				"10",
				fmt.Sprintf("--%s=%s", flags.FlagFrom, s.testnet.Validators[0].Address.String()),
				fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
				fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
				fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
			},
			false, &sdk.TxResponse{}, 0,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			clientCtx := s.testnet.Validators[0].ClientCtx
			out, err := clitestutil.ExecTestCLICmd(clientCtx, tc.cmd, tc.args)

			if tc.expectErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
				s.Require().NoError(clientCtx.JSONCodec.UnmarshalJSON(out.Bytes(), tc.respType), out.String())
				txResp := tc.respType.(*sdk.TxResponse)
				s.Require().Equal(tc.expectedCode, txResp.Code)
			}
		})
	}
}

func (s *IntegrationTestSuite) TestDeleteDistinctAccountAttributeTxCommands() {

	testCases := []struct {
		name         string
		cmd          *cobra.Command
		args         []string
		expectErr    bool
		respType     proto.Message
		expectedCode uint32
	}{
		{
			"bind a new attribute name for delete testing",
			namecli.GetBindNameCmd(),
			[]string{
				"distinct",
				s.testnet.Validators[0].Address.String(),
				"attribute",
				fmt.Sprintf("--%s=%s", flags.FlagFrom, s.testnet.Validators[0].Address.String()),
				fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
				fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
				fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
			},
			false, &sdk.TxResponse{}, 0,
		},
		{
			"add new attribute for delete testing",
			cli.NewAddAccountAttributeCmd(),
			[]string{
				"distinct.attribute",
				s.account2Addr.String(),
				"string",
				"test value",
				fmt.Sprintf("--%s=%s", flags.FlagFrom, s.testnet.Validators[0].Address.String()),
				fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
				fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
				fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
			},
			false, &sdk.TxResponse{}, 0,
		},
		{"delete distinct attribute, should fail incorrect address",
			cli.NewDeleteDistinctAccountAttributeCmd(),
			[]string{
				"distinct.attribute",
				"not-a-address",
				"string",
				"test value",
				fmt.Sprintf("--%s=%s", flags.FlagFrom, s.testnet.Validators[0].Address.String()),
				fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
				fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
				fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
			},
			true, &sdk.TxResponse{}, 0,
		},
		{"delete distinct attribute, should fail incorrect type",
			cli.NewDeleteDistinctAccountAttributeCmd(),
			[]string{
				"distinct.attribute",
				s.account2Addr.String(),
				"invalid",
				"test value",
				fmt.Sprintf("--%s=%s", flags.FlagFrom, s.testnet.Validators[0].Address.String()),
				fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
				fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
				fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
			},
			true, &sdk.TxResponse{}, 0,
		},
		{"delete distinct attribute, should successfully delete",
			cli.NewDeleteDistinctAccountAttributeCmd(),
			[]string{
				"distinct.attribute",
				s.account2Addr.String(),
				"string",
				"test value",
				fmt.Sprintf("--%s=%s", flags.FlagFrom, s.testnet.Validators[0].Address.String()),
				fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
				fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
				fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
			},
			false, &sdk.TxResponse{}, 0,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			clientCtx := s.testnet.Validators[0].ClientCtx
			out, err := clitestutil.ExecTestCLICmd(clientCtx, tc.cmd, tc.args)

			if tc.expectErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
				s.Require().NoError(clientCtx.JSONCodec.UnmarshalJSON(out.Bytes(), tc.respType), out.String())
				txResp := tc.respType.(*sdk.TxResponse)
				s.Require().Equal(tc.expectedCode, txResp.Code)
			}
		})
	}
}

func (s *IntegrationTestSuite) TestDeleteAccountAttributeTxCommands() {

	testCases := []struct {
		name         string
		cmd          *cobra.Command
		args         []string
		expectErr    bool
		respType     proto.Message
		expectedCode uint32
	}{
		{
			"bind a new attribute name for delete testing",
			namecli.GetBindNameCmd(),
			[]string{
				"deletetest",
				s.testnet.Validators[0].Address.String(),
				"attribute",
				fmt.Sprintf("--%s=%s", flags.FlagFrom, s.testnet.Validators[0].Address.String()),
				fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
				fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
				fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
			},
			false, &sdk.TxResponse{}, 0,
		},
		{
			"add new attribute for delete testing",
			cli.NewAddAccountAttributeCmd(),
			[]string{
				"deletetest.attribute",
				s.account2Addr.String(),
				"string",
				"test value",
				fmt.Sprintf("--%s=%s", flags.FlagFrom, s.testnet.Validators[0].Address.String()),
				fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
				fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
				fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
			},
			false, &sdk.TxResponse{}, 0,
		},
		{"delete attribute, should delete txtest.attribute",
			cli.NewDeleteAccountAttributeCmd(),
			[]string{
				"deletetest.attribute",
				s.account2Addr.String(),
				fmt.Sprintf("--%s=%s", flags.FlagFrom, s.testnet.Validators[0].Address.String()),
				fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
				fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
				fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
			},
			false, &sdk.TxResponse{}, 0,
		},
		{"delete attribute, should fail to find txtest.attribute",
			cli.NewDeleteAccountAttributeCmd(),
			[]string{
				"deletetest.attribute",
				s.account2Addr.String(),
				fmt.Sprintf("--%s=%s", flags.FlagFrom, s.testnet.Validators[0].Address.String()),
				fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
				fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
				fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
			},
			false, &sdk.TxResponse{}, 1,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			clientCtx := s.testnet.Validators[0].ClientCtx
			out, err := clitestutil.ExecTestCLICmd(clientCtx, tc.cmd, tc.args)

			if tc.expectErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
				s.Require().NoError(clientCtx.JSONCodec.UnmarshalJSON(out.Bytes(), tc.respType), out.String())
				txResp := tc.respType.(*sdk.TxResponse)
				s.Require().Equal(tc.expectedCode, txResp.Code)
			}
		})
	}
}

func (s *IntegrationTestSuite) TestPaginationWithPageKey() {
	asJson := fmt.Sprintf("--%s=json", tmcli.OutputFlag)

	s.T().Run("GetAccountAttribute", func(t *testing.T) {
		// Choosing page size = 35 because it a) isn't the default, b) doesn't evenly divide 500.
		pageSize := 35
		expectedCount := s.accAttrCount
		pageCount := expectedCount / pageSize
		if expectedCount%pageSize != 0 {
			pageCount++
		}
		pageSizeArg := limitArg(pageSize)

		results := make([]attributetypes.Attribute, 0, expectedCount)
		var nextKey string

		// Only using the page variable here for error messages, not for the CLI args since that'll mess with the --page-key being tested.
		for page := 1; page <= pageCount; page++ {
			// account 4 = lots of attributes with the same name but different values.
			args := []string{s.account4Str, "example.attribute.overload", pageSizeArg, asJson}
			if page != 1 {
				args = append(args, pageKeyArg(nextKey))
			}
			iterID := fmt.Sprintf("page %d/%d, args: %v", page, pageCount, args)
			cmd := cli.GetAccountAttributeCmd()
			clientCtx := s.testnet.Validators[0].ClientCtx
			out, err := clitestutil.ExecTestCLICmd(clientCtx, cmd, args)
			require.NoErrorf(t, err, "cmd error %s", iterID)
			var result attributetypes.QueryAttributeResponse
			merr := s.cfg.Codec.UnmarshalJSON(out.Bytes(), &result)
			require.NoErrorf(t, merr, "unmarshal error %s", iterID)
			resultAttrCount := len(result.Attributes)
			if page != pageCount {
				require.Equalf(t, pageSize, resultAttrCount, "page result count %s", iterID)
				require.NotEmptyf(t, result.Pagination.NextKey, "pagination next key %s", iterID)
			} else {
				require.GreaterOrEqualf(t, pageSize, resultAttrCount, "last page result count %s", iterID)
				require.Emptyf(t, result.Pagination.NextKey, "pagination next key %s", iterID)
			}
			results = append(results, result.Attributes...)
			nextKey = base64.StdEncoding.EncodeToString(result.Pagination.NextKey)
		}

		// This can fail if the --page-key isn't encoded/decoded correctly resulting in an unexpected jump forward in the actual list.
		require.Equal(t, expectedCount, len(results), "total count of attributes returned")
		// Make sure none of the results are duplicates.
		// That can happen if the --page-key isn't encoded/decoded correctly resulting in an unexpected jump backward in the actual list.
		sort.Sort(attrSorter(results))
		for i := 1; i < len(results); i++ {
			require.NotEqual(t, results[i-1], results[i], "no two attributes should be equal here")
		}
	})

	s.T().Run("ListAccountAttribute", func(t *testing.T) {
		// Choosing page size = 35 because it a) isn't the default, b) doesn't evenly divide 500.
		pageSize := 35
		expectedCount := s.accAttrCount
		pageCount := expectedCount / pageSize
		if expectedCount%pageSize != 0 {
			pageCount++
		}
		pageSizeArg := limitArg(pageSize)

		results := make([]attributetypes.Attribute, 0, expectedCount)
		var nextKey string

		// Only using the page variable here for error messages, not for the CLI args since that'll mess with the --page-key being tested.
		for page := 1; page <= pageCount; page++ {
			// account 3 = lots of attributes different names
			args := []string{s.account3Str, pageSizeArg, asJson}
			if page != 1 {
				args = append(args, pageKeyArg(nextKey))
			}
			iterID := fmt.Sprintf("page %d/%d, args: %v", page, pageCount, args)
			cmd := cli.ListAccountAttributesCmd()
			clientCtx := s.testnet.Validators[0].ClientCtx
			out, err := clitestutil.ExecTestCLICmd(clientCtx, cmd, args)
			require.NoErrorf(t, err, "cmd error %s", iterID)
			var result attributetypes.QueryAttributesResponse
			merr := s.cfg.Codec.UnmarshalJSON(out.Bytes(), &result)
			require.NoErrorf(t, merr, "unmarshal error %s", iterID)
			resultAttrCount := len(result.Attributes)
			if page != pageCount {
				require.Equalf(t, pageSize, resultAttrCount, "page result count %s", iterID)
				require.NotEmptyf(t, result.Pagination.NextKey, "pagination next key %s", iterID)
			} else {
				require.GreaterOrEqualf(t, pageSize, resultAttrCount, "last page result count %s", iterID)
				require.Emptyf(t, result.Pagination.NextKey, "pagination next key %s", iterID)
			}
			results = append(results, result.Attributes...)
			nextKey = base64.StdEncoding.EncodeToString(result.Pagination.NextKey)
		}

		// This can fail if the --page-key isn't encoded/decoded correctly resulting in an unexpected jump forward in the actual list.
		require.Equal(t, expectedCount, len(results), "total count of attributes returned")
		// Make sure none of the results are duplicates.
		// That can happen if the --page-key isn't encoded/decoded correctly resulting in an unexpected jump backward in the actual list.
		sort.Sort(attrSorter(results))
		for i := 1; i < len(results); i++ {
			require.NotEqual(t, results[i-1], results[i], "no two attributes should be equal here")
		}
	})

	s.T().Run("ScanAccountAttributesCmd different names", func(t *testing.T) {
		// Choosing page size = 35 because it a) isn't the default, b) doesn't evenly divide 48.
		// 48 comes from the number of attributes on account 3 that end with the character '7' (500/10 - "seven" - "seventeen").
		pageSize := 35
		expectedCount := 48
		pageCount := expectedCount / pageSize
		if expectedCount%pageSize != 0 {
			pageCount++
		}
		pageSizeArg := limitArg(pageSize)

		results := make([]attributetypes.Attribute, 0, expectedCount)
		var nextKey string

		// Only using the page variable here for error messages, not for the CLI args since that'll mess with the --page-key being tested.
		for page := 1; page <= pageCount; page++ {
			// account 3 = lots of attributes different names
			args := []string{s.account3Str, "7", pageSizeArg, asJson}
			if page != 1 {
				args = append(args, pageKeyArg(nextKey))
			}
			iterID := fmt.Sprintf("page %d/%d, args: %v", page, pageCount, args)
			cmd := cli.ScanAccountAttributesCmd()
			clientCtx := s.testnet.Validators[0].ClientCtx
			out, err := clitestutil.ExecTestCLICmd(clientCtx, cmd, args)
			require.NoErrorf(t, err, "cmd error %s", iterID)
			var result attributetypes.QueryScanResponse
			merr := s.cfg.Codec.UnmarshalJSON(out.Bytes(), &result)
			require.NoErrorf(t, merr, "unmarshal error %s", iterID)
			resultAttrCount := len(result.Attributes)
			if page != pageCount {
				require.Equalf(t, pageSize, resultAttrCount, "page result count %s", iterID)
				require.NotEmptyf(t, result.Pagination.NextKey, "pagination next key %s", iterID)
			} else {
				require.GreaterOrEqualf(t, pageSize, resultAttrCount, "last page result count %s", iterID)
				require.Emptyf(t, result.Pagination.NextKey, "pagination next key %s", iterID)
			}
			results = append(results, result.Attributes...)
			nextKey = base64.StdEncoding.EncodeToString(result.Pagination.NextKey)
		}

		// This can fail if the --page-key isn't encoded/decoded correctly resulting in an unexpected jump forward in the actual list.
		require.Equal(t, expectedCount, len(results), "total count of attributes returned")
		// Make sure none of the results are duplicates.
		// That can happen if the --page-key isn't encoded/decoded correctly resulting in an unexpected jump backward in the actual list.
		sort.Sort(attrSorter(results))
		for i := 1; i < len(results); i++ {
			require.NotEqual(t, results[i-1], results[i], "no two attributes should be equal here")
		}
	})

	s.T().Run("ScanAccountAttributesCmd different values", func(t *testing.T) {
		// Choosing page size = 35 because it a) isn't the default, b) doesn't evenly divide 500.
		pageSize := 35
		expectedCount := s.accAttrCount
		pageCount := expectedCount / pageSize
		if expectedCount%pageSize != 0 {
			pageCount++
		}
		pageSizeArg := limitArg(pageSize)

		results := make([]attributetypes.Attribute, 0, expectedCount)
		var nextKey string

		// Only using the page variable here for error messages, not for the CLI args since that'll mess with the --page-key being tested.
		for page := 1; page <= pageCount; page++ {
			// account 4 = lots of attributes with the same name but different values.
			args := []string{s.account4Str, "load", pageSizeArg, asJson}
			if page != 1 {
				args = append(args, pageKeyArg(nextKey))
			}
			iterID := fmt.Sprintf("page %d/%d, args: %v", page, pageCount, args)
			cmd := cli.ScanAccountAttributesCmd()
			clientCtx := s.testnet.Validators[0].ClientCtx
			out, err := clitestutil.ExecTestCLICmd(clientCtx, cmd, args)
			require.NoErrorf(t, err, "cmd error %s", iterID)
			var result attributetypes.QueryScanResponse
			merr := s.cfg.Codec.UnmarshalJSON(out.Bytes(), &result)
			require.NoErrorf(t, merr, "unmarshal error %s", iterID)
			resultAttrCount := len(result.Attributes)
			if page != pageCount {
				require.Equalf(t, pageSize, resultAttrCount, "page result count %s", iterID)
				require.NotEmptyf(t, result.Pagination.NextKey, "pagination next key %s", iterID)
			} else {
				require.GreaterOrEqualf(t, pageSize, resultAttrCount, "last page result count %s", iterID)
				require.Emptyf(t, result.Pagination.NextKey, "pagination next key %s", iterID)
			}
			results = append(results, result.Attributes...)
			nextKey = base64.StdEncoding.EncodeToString(result.Pagination.NextKey)
		}

		// This can fail if the --page-key isn't encoded/decoded correctly resulting in an unexpected jump forward in the actual list.
		require.Equal(t, expectedCount, len(results), "total count of attributes returned")
		// Make sure none of the results are duplicates.
		// That can happen if the --page-key isn't encoded/decoded correctly resulting in an unexpected jump backward in the actual list.
		sort.Sort(attrSorter(results))
		for i := 1; i < len(results); i++ {
			require.NotEqual(t, results[i-1], results[i], "no two attributes should be equal here")
		}
	})
}

type scopeAndAttrAnyMsgs struct {
	writeScopeSpecMsg    *mdtypes.MsgWriteScopeSpecificationRequest
	writeScopeSpecMsgAny *codectypes.Any
	bindNameMsg          *nametypes.MsgBindNameRequest
	bindNameMsgAny       *codectypes.Any
	writeScopeMsg        *mdtypes.MsgWriteScopeRequest
	writeScopeMsgAny     *codectypes.Any
	addAttributeMsg      *attributetypes.MsgAddAttributeRequest
	addAttributeMsgAny   *codectypes.Any
}

func (s *IntegrationTestSuite) makeScopeAndAttrMsgs() *scopeAndAttrAnyMsgs {
	rv := &scopeAndAttrAnyMsgs{
		writeScopeSpecMsg: &mdtypes.MsgWriteScopeSpecificationRequest{
			Specification: mdtypes.ScopeSpecification{
				SpecificationId: mdtypes.ScopeSpecMetadataAddress(uuid.New()),
				Description:     nil,
				OwnerAddresses:  []string{s.account1Str},
				PartiesInvolved: []mdtypes.PartyType{mdtypes.PartyType_PARTY_TYPE_OWNER},
				ContractSpecIds: []mdtypes.MetadataAddress{},
			},
			Signers:  []string{s.account1Str},
			SpecUuid: "",
		},
		bindNameMsg: &nametypes.MsgBindNameRequest{
			Parent: nametypes.NameRecord{
				Name:       "pb",
				Address:    s.account5Str,
				Restricted: false,
			},
			Record: nametypes.NameRecord{
				Name:       "jake",
				Address:    s.account5Str,
				Restricted: false,
			},
		},
	}
	rv.writeScopeMsg = &mdtypes.MsgWriteScopeRequest{
		Scope: mdtypes.Scope{
			ScopeId:           mdtypes.ScopeMetadataAddress(uuid.New()),
			SpecificationId:   rv.writeScopeSpecMsg.Specification.SpecificationId,
			Owners:            []mdtypes.Party{{s.account5Str, mdtypes.PartyType_PARTY_TYPE_OWNER}},
			DataAccess:        []string{s.account4Str, s.account5Str},
			ValueOwnerAddress: s.account5Str,
		},
		Signers:   []string{s.account5Str},
		ScopeUuid: "",
		SpecUuid:  "",
	}
	rv.addAttributeMsg = &attributetypes.MsgAddAttributeRequest{
		Name:          rv.bindNameMsg.Record.Name + "." + rv.bindNameMsg.Parent.Name,
		Value:         []byte("test"),
		AttributeType: attributetypes.AttributeType_String,
		Account:       rv.writeScopeMsg.Scope.ScopeId.String(),
		Owner:         s.account5Str,
	}
	return rv
}

func (a *scopeAndAttrAnyMsgs) populateAnyFields() error {
	var err error
	a.writeScopeSpecMsgAny, err = codectypes.NewAnyWithValue(a.writeScopeSpecMsg)
	if err != nil {
		return fmt.Errorf("Could not wrap %s as Any: %w", "MsgWriteScopeSpecificationRequest", err)
	}
	a.bindNameMsgAny, err = codectypes.NewAnyWithValue(a.bindNameMsg)
	if err != nil {
		return fmt.Errorf("Could not wrap %s as Any: %w", "MsgBindNameRequest", err)
	}
	a.writeScopeMsgAny, err = codectypes.NewAnyWithValue(a.writeScopeMsg)
	if err != nil {
		return fmt.Errorf("Could not wrap %s as Any: %w", "MsgWriteScopeRequest", err)
	}
	a.addAttributeMsgAny, err = codectypes.NewAnyWithValue(a.addAttributeMsg)
	if err != nil {
		return fmt.Errorf("Could not wrap %s as Any: %w", "MsgAddAttributeRequest", err)
	}
	return nil
}

func (s *IntegrationTestSuite) MakeTx(msgs ...*codectypes.Any) tx.Tx {
	return tx.Tx{
		Body: &tx.TxBody{
			Messages:                    msgs,
			Memo:                        "",
			TimeoutHeight:               0,
			ExtensionOptions:            []*codectypes.Any{},
			NonCriticalExtensionOptions: []*codectypes.Any{},
		},
		AuthInfo: &tx.AuthInfo{
			SignerInfos: []*tx.SignerInfo{},
			Fee: &tx.Fee{
				Amount:   sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))),
				GasLimit: 200000,
				Payer:    "",
				Granter:  "",
			},
		},
		Signatures: nil,
	}
}

func (s *IntegrationTestSuite) TestScopeAndAttributeTx() {
	ctx := s.testnet.Validators[0].ClientCtx.WithKeyringDir(s.keyringDir).WithKeyring(s.keyring)
	dir := s.T().TempDir()

	msgs := s.makeScopeAndAttrMsgs()
	err := msgs.populateAnyFields()
	s.Require().NoError(err, "making messages")

	scopeSpecTx := s.MakeTx(msgs.writeScopeSpecMsgAny)
	_, err = s.SendAndCheckTx(ctx, scopeSpecTx, s.account1Str, "1-scopeSpecTx", dir)
	s.Require().NoError(err)

	nameTx := s.MakeTx(msgs.bindNameMsgAny)
	_, err = s.SendAndCheckTx(ctx, nameTx, s.account5Str, "2-nameTx", dir)
	s.Require().NoError(err)

	scopeAndAttrTx := s.MakeTx(msgs.writeScopeMsgAny, msgs.addAttributeMsgAny)
	_, err = s.SendAndCheckTx(ctx, scopeAndAttrTx, s.account5Str, "3-scopeAndAttrTx", dir)
	s.Require().NoError(err)
	// Pro tip: SendAndCheck saves several files while running.
	// To view them, put a break point on the above line, then debug this test.
	// Once it hits that breakpoint, get the dir value and copy all the files
	// somewhere more permanent.
}

func (s *IntegrationTestSuite) TestPb1ScopeAndAttributeTx() {
	ctx := s.testnet.Validators[0].ClientCtx.WithKeyringDir(s.keyringDir).WithKeyring(s.keyring)
	dir := s.T().TempDir()

	msgs := s.makeScopeAndAttrMsgs()
	msgs.bindNameMsg.Parent.Name = "pb1"
	msgs.bindNameMsg.Parent.Address = s.account1Str
	msgs.addAttributeMsg.Name = "jake.pb1"
	err := msgs.populateAnyFields()
	s.Require().NoError(err, "making messages")

	scopeSpecTx := s.MakeTx(msgs.writeScopeSpecMsgAny)
	_, err = s.SendAndCheckTx(ctx, scopeSpecTx, s.account1Str, "1-scopeSpecTx", dir)
	s.Require().NoError(err)

	nameTx := s.MakeTx(msgs.bindNameMsgAny)
	_, err = s.SendAndCheckTx(ctx, nameTx, s.account1Str, "2-nameTx", dir)
	s.Require().NoError(err)

	scopeAndAttrTx := s.MakeTx(msgs.writeScopeMsgAny, msgs.addAttributeMsgAny)
	_, err = s.SendAndCheckTx(ctx, scopeAndAttrTx, s.account5Str, "3-scopeAndAttrTx", dir)
	s.Require().NoError(err)
	// Pro tip: SendAndCheck saves several files while running.
	// To view them, put a break point on the above line, then debug this test.
	// Once it hits that breakpoint, get the dir value and copy all the files
	// somewhere more permanent.
}

func (s *IntegrationTestSuite) SendAndCheckTx(ctx client.Context, toSend tx.Tx, signerAddr, txName, dir string) (*sdk.TxResponse, error) {
	fc := 0
	txJSON, err := s.testnet.Config.TxConfig.TxJSONEncoder()(&toSend)
	if err != nil {
		return nil, fmt.Errorf("could not encode %q tx as JSON: %w", txName, err)
	}
	s.T().Logf("%q using directory %s", txName, dir)
	fc++
	unsignedFile := filepath.Join(dir, fmt.Sprintf("%s-%d-unsigned.json", txName, fc))
	err = os.WriteFile(unsignedFile, txJSON, 0600)
	if err != nil {
		return nil, fmt.Errorf("could not write %q unsigned file: %w", txName, err)
	}
	sbArgs := []string{unsignedFile,
		fmt.Sprintf("--%s=%s", flags.FlagChainID, ctx.ChainID),
		fmt.Sprintf("--%s=%s", flags.FlagFrom, signerAddr),
	}
	signedJSON, err := clitestutil.ExecTestCLICmd(ctx, authcli.GetSignCommand(), sbArgs)
	if err != nil {
		return nil, fmt.Errorf("could not sign %q tx %q: %w", txName, sbArgs, err)
	}
	signedJSONBz := signedJSON.Bytes()
	s.T().Logf("%q tx input:\n%s\n", txName, signedJSONBz)
	fc++
	signedFile := filepath.Join(dir, fmt.Sprintf("%s-%d-signed.json", txName, fc))
	err = os.WriteFile(signedFile, signedJSONBz, 0600)
	if err != nil {
		return nil, fmt.Errorf("could not write %q signed file: %w", txName, err)
	}
	bArgs := []string{signedFile,
		"-o", "json",
	}
	bOut, err := clitestutil.ExecTestCLICmd(ctx, authcli.GetBroadcastCommand(), bArgs)
	if err != nil {
		return nil, fmt.Errorf("could not broadcast %q tx %q: %w", txName, bArgs, err)
	}
	bOutBz := bOut.Bytes()
	s.T().Logf("%q broadcast output:\n%s\n", txName, bOutBz)
	fc++
	err = os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s-%d-broadcast-output.json", txName, fc)), bOutBz, 0600)
	if err != nil {
		return nil, fmt.Errorf("could not write %q tx output file: %w", txName, err)
	}

	bTxResp := sdk.TxResponse{}
	err = s.testnet.Config.Codec.UnmarshalJSON(bOutBz, &bTxResp)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal broadcast %q tx response: %w", txName, err)
	}
	if uint32(0) != bTxResp.Code {
		return nil, fmt.Errorf("broadcast %q tx returned a code %d (expected 0)", txName, bTxResp.Code)
	}

	err = s.WaitForBlocks(txName, 2)
	if err != nil {
		return nil, err
	}

	qTxArgs := []string{"--type=hash", bTxResp.TxHash, "-o", "json"}
	qTxOut, err := clitestutil.ExecTestCLICmd(ctx, authcli.QueryTxCmd(), qTxArgs)
	if err != nil {
		return nil, fmt.Errorf("could not query %q tx %q: %w", txName, qTxArgs, err)
	}
	qTxOutBz := qTxOut.Bytes()
	s.T().Logf("%q query result:\n%s\n", txName, qTxOutBz)
	fc++
	err = os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s-%d-tx-response.json", txName, fc)), qTxOutBz, 0600)
	if err != nil {
		return nil, fmt.Errorf("could not write %q tx response file: %w", txName, err)
	}

	qTxResp := sdk.TxResponse{}
	err = s.testnet.Config.Codec.UnmarshalJSON(qTxOutBz, &qTxResp)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal query %q tx response: %w", txName, err)
	}
	if uint32(0) != qTxResp.Code {
		return nil, fmt.Errorf("query %q tx returned a code %d (expected 0)", txName, qTxResp.Code)
	}
	return &qTxResp, nil
}

func (s IntegrationTestSuite) WaitForBlocks(txName string, count int) error {
	for i := 1; i <= count; i++ {
		s.T().Logf("%q waiting for block %d of %d.", txName, i, count)
		err := s.testnet.WaitForNextBlock()
		if err != nil {
			return fmt.Errorf("could not wait for block %d of %d during %q tx: %w", i, count, txName, err)
		}
	}
	return nil
}
