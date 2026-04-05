package tui

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/evertras/bubble-table/table"

	"github.com/hairglasses-studio/runmylife/internal/tui/components"
)

type financeData struct {
	MTDSpend     float64
	LastMTDSpend float64
	Budgets      []budgetStatus
	Transactions []txnItem
	DailySpend   []float64
}

type budgetStatus struct {
	Category  string
	Limit     float64
	Spent     float64
	Remaining float64
}

type txnItem struct {
	Date        string
	Description string
	Amount      float64
	Category    string
}

func loadFinanceData(db *sql.DB) financeData {
	ctx := context.Background()
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	lastMonthStart := time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	lastMonthSameDay := time.Date(now.Year(), now.Month()-1, now.Day(), 0, 0, 0, 0, now.Location()).Format("2006-01-02")

	d := financeData{}

	// MTD spend
	db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(ABS(amount)), 0) FROM transactions
		 WHERE type = 'expense' AND date >= ?`, monthStart,
	).Scan(&d.MTDSpend)

	// Last month's MTD spend (same point in month for comparison)
	db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(ABS(amount)), 0) FROM transactions
		 WHERE type = 'expense' AND date >= ? AND date <= ?`,
		lastMonthStart, lastMonthSameDay,
	).Scan(&d.LastMTDSpend)

	// Budgets
	rows, err := db.QueryContext(ctx,
		`SELECT category, monthly_limit FROM budgets`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var bs budgetStatus
			if rows.Scan(&bs.Category, &bs.Limit) == nil {
				db.QueryRowContext(ctx,
					`SELECT COALESCE(SUM(ABS(amount)), 0) FROM transactions
					 WHERE date >= ? AND category = ? AND type = 'expense'`,
					monthStart, bs.Category,
				).Scan(&bs.Spent)
				bs.Remaining = bs.Limit - bs.Spent
				d.Budgets = append(d.Budgets, bs)
			}
		}
	}

	// Recent transactions
	rows, err = db.QueryContext(ctx,
		`SELECT date, COALESCE(description, ''), amount, COALESCE(category, '')
		 FROM transactions ORDER BY date DESC LIMIT 10`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var t txnItem
			if rows.Scan(&t.Date, &t.Description, &t.Amount, &t.Category) == nil {
				d.Transactions = append(d.Transactions, t)
			}
		}
	}

	// 30-day daily spend for sparkline
	spendRows, err := db.QueryContext(ctx,
		`SELECT date(date), COALESCE(SUM(ABS(amount)), 0)
		 FROM transactions
		 WHERE type = 'expense' AND date >= date('now', '-30 days')
		 GROUP BY date(date) ORDER BY date(date)`)
	if err == nil {
		defer spendRows.Close()
		for spendRows.Next() {
			var dt string
			var amt float64
			if spendRows.Scan(&dt, &amt) == nil {
				d.DailySpend = append(d.DailySpend, amt)
			}
		}
	}

	return d
}

func renderFinances(d financeData, width int) string {
	var b strings.Builder

	// MTD spend with trend arrow
	mtd := components.StyledIcon(components.IconDollar, colorSuccess) + subtitleStyle.Render("Month-to-Date") + "\n"
	amountStr := fmt.Sprintf("$%.2f", d.MTDSpend)
	mtd += "  " + lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).Render(amountStr)

	if d.LastMTDSpend > 0 {
		diff := d.MTDSpend - d.LastMTDSpend
		pct := diff / d.LastMTDSpend * 100
		if diff > 0 {
			mtd += "  " + alertStyle.Render(fmt.Sprintf("%s+%.0f%% vs last month", components.IconTrendUp, pct))
		} else if diff < 0 {
			mtd += "  " + successStyle.Render(fmt.Sprintf("%s%.0f%% vs last month", components.IconTrendDown, pct))
		}
	}
	b.WriteString(cardStyle.Width(width).Render(mtd))
	b.WriteString("\n")

	// Budget gauges
	if len(d.Budgets) > 0 {
		budgets := components.StyledIcon(components.IconBudget, colorSecondary) + subtitleStyle.Render("Budgets") + "\n"
		barWidth := 20
		if width < 60 {
			barWidth = 15
		}
		for _, bs := range d.Budgets {
			gauge := components.Gauge(bs.Spent, bs.Limit, barWidth,
				components.WithLabel(fmt.Sprintf("  %-12s", bs.Category)),
				components.WithThresholds(0.7, 0.9),
			)

			remaining := bs.Remaining
			remStr := successStyle.Render(fmt.Sprintf("$%.0f left", remaining))
			if remaining < 0 {
				remStr = alertStyle.Render(fmt.Sprintf("$%.0f over!", math.Abs(remaining)))
			}
			budgets += gauge + "  " + remStr + "\n"
		}
		b.WriteString(cardStyle.Width(width).Render(budgets))
		b.WriteString("\n")
	}

	// 30-day spend sparkline
	if len(d.DailySpend) > 5 {
		spark := subtitleStyle.Render("30-Day Spend Trend") + "\n"
		sparkWidth := width - 8
		if sparkWidth > 60 {
			sparkWidth = 60
		}
		spark += components.Sparkline(d.DailySpend, sparkWidth, colorSeries1)
		b.WriteString(cardStyle.Width(width).Render(spark))
		b.WriteString("\n")
	}

	// Transaction table
	if len(d.Transactions) > 0 {
		txns := subtitleStyle.Render("Recent Transactions") + "\n"
		cols := []table.Column{
			table.NewColumn("date", "Date", 12),
			table.NewFlexColumn("desc", "Description", 1),
			table.NewColumn("cat", "Category", 12),
			table.NewColumn("amt", "Amount", 12),
		}
		var rows []table.Row
		for _, t := range d.Transactions {
			desc := t.Description
			if len(desc) > 30 {
				desc = desc[:27] + "..."
			}
			amtStr := fmt.Sprintf("$%.2f", math.Abs(t.Amount))
			if t.Amount >= 0 {
				amtStr = successStyle.Render("+" + amtStr)
			}
			rows = append(rows, table.NewRow(table.RowData{
				"date": t.Date,
				"desc": desc,
				"cat":  t.Category,
				"amt":  amtStr,
			}))
		}
		tbl := components.SimpleTable(cols, rows, width-4)
		txns += tbl.View()
		b.WriteString(cardStyle.Width(width).Render(txns))
	}

	return b.String()
}
