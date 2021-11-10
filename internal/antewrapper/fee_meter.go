package antewrapper

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/armon/go-metrics"
	"github.com/tendermint/tendermint/libs/log"

	sdkgas "github.com/cosmos/cosmos-sdk/store/types"
	"github.com/cosmos/cosmos-sdk/telemetry"
)

type FeeGasMeter struct {
	// a context logger reference for info/debug output
	log log.Logger
	// the gas meter being wrapped
	base sdkgas.GasMeter
	// tracks amount used per purpose
	used map[string]uint64
	// tracks number of usages per purpose
	calls map[string]uint64

	usedFees map[string]sdk.Coin // map of msg fee type url --> fees charged

	feesWanted map[string]sdk.Coin // map of msg fee type url --> fees wanted

	// in passing cases usedFees and feesWanted should match
}

// NewFeeTracingMeterWrapper returns a reference to a new tracing gas meter that will track calls to the base gas meter
func NewFeeTracingMeterWrapper(logger log.Logger, baseMeter sdkgas.GasMeter) sdkgas.GasMeter {
	return &FeeGasMeter{
		log:        logger,
		base:       baseMeter,
		used:       make(map[string]uint64),
		calls:      make(map[string]uint64),
		usedFees:   make(map[string]sdk.Coin),
		feesWanted: make(map[string]sdk.Coin),
	}
}

var _ sdkgas.GasMeter = &FeeGasMeter{}

// GasConsumed reports the amount of gas consumed at Log.Info level
func (g *FeeGasMeter) GasConsumed() sdkgas.Gas {
	return g.base.GasConsumed()
}

// RefundGas refunds an amount of gas
func (g *FeeGasMeter) RefundGas(amount uint64, descriptor string) {
	g.base.RefundGas(amount, descriptor)
}

// GasConsumedToLimit will report the actual consumption or the meter limit, whichever is less.
func (g *FeeGasMeter) GasConsumedToLimit() sdkgas.Gas {
	return g.base.GasConsumedToLimit()
}

// Limit for amount of gas that can be consumed (if zero then unlimited)
func (g *FeeGasMeter) Limit() sdkgas.Gas {
	return g.base.Limit()
}

// ConsumeGas increments the amount of gas used on the meter associated with a given purpose.
func (g *FeeGasMeter) ConsumeGas(amount sdkgas.Gas, descriptor string) {
	cur := g.used[descriptor]
	g.used[descriptor] = cur + amount

	cur = g.calls[descriptor]
	g.calls[descriptor] = cur + 1

	telemetry.IncrCounterWithLabels([]string{"tx", "gas", "consumed"}, float32(amount), []metrics.Label{telemetry.NewLabel("purpose", descriptor)})

	g.base.ConsumeGas(amount, descriptor)
}

// IsPastLimit indicates consumption has passed the limit (if any)
func (g *FeeGasMeter) IsPastLimit() bool {
	return g.base.IsPastLimit()
}

// IsOutOfGas indicates the gas meter has tracked consumption at or above the limit
func (g *FeeGasMeter) IsOutOfGas() bool {
	return g.base.IsOutOfGas()
}

// String implements stringer interface
func (g *FeeGasMeter) String() string {
	return fmt.Sprintf("tracingGasMeter:\n  limit: %d\n  consumed: %d", g.base.Limit(), g.base.GasConsumed())
}

// ConsumeFee increments the amount of gas used on the meter associated with a given purpose.
func (g *FeeGasMeter) ConsumeFee(amount sdk.Coin, msgType string) {
	cur := g.usedFees[msgType]
	g.usedFees[msgType] = cur.Add(amount)
}

// FeeWanted increments the additional fee count based on msgType
func (g *FeeGasMeter) FeeWanted(amount sdk.Coin, msgType string) {
	cur := g.feesWanted[msgType]
	g.feesWanted[msgType] = cur.Add(amount)
}
