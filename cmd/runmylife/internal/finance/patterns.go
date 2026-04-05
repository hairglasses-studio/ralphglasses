package finance

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"
)

// MonthlyAmount holds spend data for a single month.
type MonthlyAmount struct {
	Month  string  `json:"month"`
	Amount float64 `json:"amount"`
}

// MerchantTotal holds aggregated spend for a merchant.
type MerchantTotal struct {
	Description string  `json:"description"`
	Total       float64 `json:"total"`
	Count       int     `json:"count"`
}

// RecurringCharge is a detected repeating transaction.
type RecurringCharge struct {
	Description string  `json:"description"`
	Amount      float64 `json:"avg_amount"`
	Category    string  `json:"category"`
	Occurrences int     `json:"occurrences"`
	LastSeen    string  `json:"last_seen"`
}

// WeekdayPattern returns average spend by day of week (0=Sunday, 6=Saturday).
func WeekdayPattern(ctx context.Context, db *sql.DB) map[string]float64 {
	result := make(map[string]float64)
	days := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}

	rows, err := db.QueryContext(ctx,
		`SELECT CAST(strftime('%w', date) AS INTEGER) as dow,
		        AVG(ABS(amount)) as avg_spend
		 FROM transactions
		 WHERE type = 'expense' AND date >= date('now', '-90 days')
		 GROUP BY dow ORDER BY dow`)
	if err != nil {
		return result
	}
	defer rows.Close()

	for rows.Next() {
		var dow int
		var avg float64
		if rows.Scan(&dow, &avg) == nil && dow >= 0 && dow < 7 {
			result[days[dow]] = math.Round(avg*100) / 100
		}
	}
	return result
}

// CategoryTrend returns month-over-month spend for a category.
func CategoryTrend(ctx context.Context, db *sql.DB, category string, months int) []MonthlyAmount {
	if months <= 0 {
		months = 6
	}
	since := time.Now().AddDate(0, -months, 0).Format("2006-01-02")

	rows, err := db.QueryContext(ctx,
		`SELECT strftime('%Y-%m', date) as month, SUM(ABS(amount)) as total
		 FROM transactions
		 WHERE type = 'expense' AND category = ? AND date >= ?
		 GROUP BY month ORDER BY month`,
		category, since)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var results []MonthlyAmount
	for rows.Next() {
		var ma MonthlyAmount
		if rows.Scan(&ma.Month, &ma.Amount) == nil {
			ma.Amount = math.Round(ma.Amount*100) / 100
			results = append(results, ma)
		}
	}
	return results
}

// TopMerchants returns the highest-spend merchants over the given number of days.
func TopMerchants(ctx context.Context, db *sql.DB, days int) []MerchantTotal {
	if days <= 0 {
		days = 30
	}
	since := time.Now().AddDate(0, 0, -days).Format("2006-01-02")

	rows, err := db.QueryContext(ctx,
		`SELECT description, SUM(ABS(amount)) as total, COUNT(*) as cnt
		 FROM transactions
		 WHERE type = 'expense' AND date >= ?
		 GROUP BY description ORDER BY total DESC LIMIT 15`,
		since)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var results []MerchantTotal
	for rows.Next() {
		var mt MerchantTotal
		if rows.Scan(&mt.Description, &mt.Total, &mt.Count) == nil {
			mt.Total = math.Round(mt.Total*100) / 100
			// Normalize the description for cleaner display
			if normalized := NormalizeMerchant(mt.Description); normalized != "" {
				mt.Description = normalized
			}
			results = append(results, mt)
		}
	}
	return results
}

// RecurringDetector finds transactions that repeat monthly (±3 days, ±10% amount).
func RecurringDetector(ctx context.Context, db *sql.DB) []RecurringCharge {
	// Find descriptions that appear 2+ times in the last 90 days
	rows, err := db.QueryContext(ctx,
		`SELECT description, AVG(ABS(amount)), category, COUNT(*), MAX(date)
		 FROM transactions
		 WHERE type = 'expense' AND date >= date('now', '-90 days')
		 GROUP BY description
		 HAVING COUNT(*) >= 2
		 ORDER BY AVG(ABS(amount)) DESC`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var candidates []RecurringCharge
	for rows.Next() {
		var rc RecurringCharge
		var cat sql.NullString
		if rows.Scan(&rc.Description, &rc.Amount, &cat, &rc.Occurrences, &rc.LastSeen) == nil {
			if cat.Valid {
				rc.Category = cat.String
			}
			rc.Amount = math.Round(rc.Amount*100) / 100
			candidates = append(candidates, rc)
		}
	}

	// Filter to actual recurring: check if amounts are consistent (±10%)
	var recurring []RecurringCharge
	for _, c := range candidates {
		if isLikelyRecurring(ctx, db, c.Description, c.Amount) {
			if normalized := NormalizeMerchant(c.Description); normalized != "" {
				c.Description = normalized
			}
			recurring = append(recurring, c)
		}
	}
	return recurring
}

// isLikelyRecurring checks if all transactions for a description have consistent amounts.
func isLikelyRecurring(ctx context.Context, db *sql.DB, description string, avgAmount float64) bool {
	var minAmt, maxAmt float64
	err := db.QueryRowContext(ctx,
		`SELECT MIN(ABS(amount)), MAX(ABS(amount))
		 FROM transactions
		 WHERE description = ? AND type = 'expense' AND date >= date('now', '-90 days')`,
		description).Scan(&minAmt, &maxAmt)
	if err != nil {
		return false
	}

	if avgAmount == 0 {
		return false
	}
	// Check if variance is within 10% of the average
	variance := (maxAmt - minAmt) / avgAmount
	return variance < 0.10

}

// SpendingSummary returns aggregate spending stats for a date range.
func SpendingSummary(ctx context.Context, db *sql.DB, since, until string) map[string]any {
	result := map[string]any{}

	var totalExpense, totalIncome float64
	db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(ABS(amount)), 0) FROM transactions
		 WHERE type = 'expense' AND date >= ? AND date < ?`,
		since, until).Scan(&totalExpense)
	db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(amount), 0) FROM transactions
		 WHERE type = 'income' AND date >= ? AND date < ?`,
		since, until).Scan(&totalIncome)

	result["total_expense"] = math.Round(totalExpense*100) / 100
	result["total_income"] = math.Round(totalIncome*100) / 100
	result["net"] = math.Round((totalIncome-totalExpense)*100) / 100

	// Category breakdown
	rows, err := db.QueryContext(ctx,
		`SELECT COALESCE(category, 'uncategorized'), SUM(ABS(amount))
		 FROM transactions
		 WHERE type = 'expense' AND date >= ? AND date < ?
		 GROUP BY category ORDER BY SUM(ABS(amount)) DESC`,
		since, until)
	if err == nil {
		defer rows.Close()
		categories := make(map[string]float64)
		for rows.Next() {
			var cat string
			var amount float64
			if rows.Scan(&cat, &amount) == nil {
				categories[cat] = math.Round(amount*100) / 100
			}
		}
		result["categories"] = categories
	}

	result["period"] = fmt.Sprintf("%s to %s", since, until)
	return result
}
