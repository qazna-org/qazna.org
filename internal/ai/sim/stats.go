package sim

type Counter struct {
	Transfers       int
	TotalMinorUnits int64
	Currency        string
}

func (c *Counter) Add(t Transfer) {
	c.Transfers++
	c.TotalMinorUnits += t.Amount
	if c.Currency == "" {
		c.Currency = t.Currency
	}
}

func (c Counter) MajorAmount() float64 {
	return float64(c.TotalMinorUnits) / 100
}
