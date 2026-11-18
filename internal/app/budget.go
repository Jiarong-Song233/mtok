package app

import (
	"time"

	"github.com/Jiarong-Song233/mtok/internal/agg"
	"github.com/Jiarong-Song233/mtok/internal/render"
)

// budgetStatus compares today's and this month's reference spend with the
// configured limits. It uses the same magpie-exclusion rule as the main table,
// so routed proxy rows cannot be counted twice.
func (e *env) budgetStatus(o options, now time.Time) *render.Budget {
	daily, monthly := e.set.BudgetDaily, e.set.BudgetMonthly
	cur := e.set.Currency
	if cur == "" {
		cur = "CNY"
	}
	if daily <= 0 && monthly <= 0 {
		if !o.check {
			return nil
		}
		return &render.Budget{
			Currency: cur,
			OK:       true,
			Note:     "未设置预算：在 ~/.config/mtok/config.yaml 里写 budget.daily / budget.monthly",
		}
	}

	rows := e.store.Rows()
	opt := agg.Options{
		ExcludeMagpie: o.noMagpie || !e.set.Magpie,
		Price:         e.priceFn(),
	}
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	dayStr := day.Format("2006-01-02")
	rate := e.displayRate()

	spend := func(since string) float64 {
		opt.Since, opt.Until = since, dayStr
		return agg.Build(rows, opt).Totals.CostUSD * rate
	}

	b := &render.Budget{Currency: cur, OK: true}
	if daily > 0 {
		spent := spend(dayStr)
		b.Daily = &render.BudgetPeriod{Limit: daily, Spent: spent, Currency: cur, Exceeded: spent > daily}
		if b.Daily.Exceeded {
			b.OK = false
		}
	}
	if monthly > 0 {
		spent := spend(monthStart.Format("2006-01-02"))
		b.Monthly = &render.BudgetPeriod{Limit: monthly, Spent: spent, Currency: cur, Exceeded: spent > monthly}
		if b.Monthly.Exceeded {
			b.OK = false
		}
	}
	return b
}
