package shipping

import (
	"fmt"
	"math"
)

// Quote represents a currency value.
type Quote struct {
	Dollars uint32
	Cents   uint32
}

// String representation of the Quote.
func (q Quote) String() string {
	return fmt.Sprintf("$%d.%d", q.Dollars, q.Cents)
}

// QuoteFromCount takes a number of items and returns a shipping quote.
// Flat rate: $8.99 for any non-empty shipment, $0 for empty.
func QuoteFromCount(count int) Quote {
	if count == 0 {
		return QuoteFromFloat(0)
	}
	return QuoteFromFloat(8.99)
}

// QuoteFromFloat takes a price as a float and creates a Quote.
func QuoteFromFloat(value float64) Quote {
	units, fraction := math.Modf(value)
	return Quote{
		Dollars: uint32(units),
		Cents:   uint32(math.Trunc(fraction * 100)),
	}
}
