package sim

import (
	"math/rand"
	"time"
)

type Account struct {
	ID       string
	Currency string
	Label    string
	Initial  int64
}

type Transfer struct {
	FromID    string
	ToID      string
	Amount    int64
	Currency  string
	Narrative string
}

type Scenario struct {
	Name       string
	Accounts   []Account
	Narratives []string
}

func SovereignFlowScenario() Scenario {
	return Scenario{
		Name: "SovereignReserveDelta",
		Accounts: []Account{
			{ID: "acct-sovereign-001", Currency: "QZN", Label: "National Bank of Qazakhstan", Initial: 10_000_000_000},
			{ID: "acct-sovereign-002", Currency: "QZN", Label: "Union Reserve Cooperative", Initial: 7_500_000_000},
			{ID: "acct-sovereign-003", Currency: "USD", Label: "European Monetary Authority", Initial: 2_000_000_000},
		},
		Narratives: []string{
			"Intra-day rebalancing to demonstrate liquidity automation",
			"FX swap to stabilize regional currency corridor",
			"Cross-border settlement spike after macro announcement",
		},
	}
}

type Generator struct {
	scenario Scenario
	rnd      *rand.Rand
}

func NewGenerator(seed int64) Generator {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	return Generator{scenario: SovereignFlowScenario(), rnd: rand.New(rand.NewSource(seed))}
}

func (g Generator) NextTransfer() Transfer {
	accs := g.scenario.Accounts
	if len(accs) < 2 {
		panic("scenario requires >=2 accounts")
	}
	fromIdx := g.rnd.Intn(len(accs))
	toIdx := g.rnd.Intn(len(accs) - 1)
	if toIdx >= fromIdx {
		toIdx++
	}
	from := accs[fromIdx]
	to := accs[toIdx]
	// Ensure currency alignment: if mismatch, convert heuristically.
	currency := from.Currency
	amount := int64(g.rnd.Intn(900_000)+100_000) * 100 // 0.1M - 1.0M major units
	narrative := g.scenario.Narratives[g.rnd.Intn(len(g.scenario.Narratives))]
	return Transfer{
		FromID:    from.ID,
		ToID:      to.ID,
		Currency:  currency,
		Amount:    amount,
		Narrative: narrative,
	}
}

func (g Generator) Accounts() []Account {
	return append([]Account(nil), g.scenario.Accounts...)
}

func (g *Generator) OverrideAccounts(accounts []Account) {
	g.scenario.Accounts = append([]Account(nil), accounts...)
}
