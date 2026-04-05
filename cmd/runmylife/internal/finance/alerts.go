package finance

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"
)

// AlertType classifies the kind of spending anomaly.
type AlertType string

const (
	AlertLargeTransaction   AlertType = "large_transaction"
	AlertHighDailySpend     AlertType = "high_daily_spend"
	AlertNewLargeMerchant   AlertType = "new_large_merchant"
	AlertSubscriptionChange AlertType = "subscription_change"
)

// Severity indicates how important the alert is.
type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityAlert   Severity = "alert"
)

// SpendingAlert describes a detected spending anomaly.
type SpendingAlert struct {
	Type        AlertType `json:"type"`
	Severity    Severity  `json:"severity"`
	Message     string    `json:"message"`
	Amount      float64   `json:"amount"`
	Description string    `json:"description"`
	DetectedAt  string    `json:"detected_at"`
}

// DetectAnomalies scans recent transactions for spending anomalies.
func DetectAnomalies(ctx context.Context, db *sql.DB) []SpendingAlert {
	var alerts []SpendingAlert
	now := time.Now().Format(time.RFC3339)

	alerts = append(alerts, detectLargeTransactions(ctx, db, now)...)
	alerts = append(alerts, detectHighDailySpend(ctx, db, now)...)
	alerts = append(alerts, detectNewLargeMerchants(ctx, db, now)...)
	alerts = append(alerts, detectSubscriptionChanges(ctx, db, now)...)

	return alerts
}

// detectLargeTransactions finds single transactions >2x category average.
func detectLargeTransactions(ctx context.Context, db *sql.DB, now string) []SpendingAlert {
	// Get category averages from last 60 days
	avgRows, err := db.QueryContext(ctx,
		`SELECT category, AVG(ABS(amount)) FROM transactions
		 WHERE type = 'expense' AND date >= date('now', '-60 days')
		 GROUP BY category`)
	if err != nil {
		return nil
	}
	defer avgRows.Close()

	catAvg := make(map[string]float64)
	for avgRows.Next() {
		var cat string
		var avg float64
		if avgRows.Scan(&cat, &avg) == nil {
			catAvg[cat] = avg
		}
	}

	// Check last 7 days for outliers
	rows, err := db.QueryContext(ctx,
		`SELECT description, ABS(amount), category, date FROM transactions
		 WHERE type = 'expense' AND date >= date('now', '-7 days')
		 ORDER BY ABS(amount) DESC`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var alerts []SpendingAlert
	for rows.Next() {
		var desc, cat, date string
		var amount float64
		if rows.Scan(&desc, &amount, &cat, &date) == nil {
			avg, ok := catAvg[cat]
			if ok && avg > 0 && amount > avg*2 {
				severity := SeverityWarning
				if amount > avg*5 {
					severity = SeverityAlert
				}
				normalized := NormalizeMerchant(desc)
				alerts = append(alerts, SpendingAlert{
					Type:        AlertLargeTransaction,
					Severity:    severity,
					Message:     fmt.Sprintf("$%.2f at %s is %.1fx your average %s spend ($%.2f)", amount, normalized, amount/avg, cat, avg),
					Amount:      math.Round(amount*100) / 100,
					Description: normalized,
					DetectedAt:  now,
				})
			}
		}
	}
	return alerts
}

// detectHighDailySpend checks if today's spend exceeds 1.5x the 30-day daily average.
func detectHighDailySpend(ctx context.Context, db *sql.DB, now string) []SpendingAlert {
	var dailyAvg float64
	err := db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(ABS(amount)), 0) / 30.0 FROM transactions
		 WHERE type = 'expense' AND date >= date('now', '-30 days')`).Scan(&dailyAvg)
	if err != nil || dailyAvg == 0 {
		return nil
	}

	var todaySpend float64
	db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(ABS(amount)), 0) FROM transactions
		 WHERE type = 'expense' AND date = date('now')`).Scan(&todaySpend)

	if todaySpend > dailyAvg*1.5 {
		severity := SeverityWarning
		if todaySpend > dailyAvg*3 {
			severity = SeverityAlert
		}
		return []SpendingAlert{{
			Type:       AlertHighDailySpend,
			Severity:   severity,
			Message:    fmt.Sprintf("Today's spend ($%.2f) is %.1fx your daily average ($%.2f)", todaySpend, todaySpend/dailyAvg, dailyAvg),
			Amount:     math.Round(todaySpend*100) / 100,
			DetectedAt: now,
		}}
	}
	return nil
}

// detectNewLargeMerchants finds first-time merchants with charges over $50.
func detectNewLargeMerchants(ctx context.Context, db *sql.DB, now string) []SpendingAlert {
	rows, err := db.QueryContext(ctx,
		`SELECT t1.description, ABS(t1.amount), t1.date
		 FROM transactions t1
		 WHERE t1.type = 'expense'
		   AND t1.date >= date('now', '-7 days')
		   AND ABS(t1.amount) > 50
		   AND NOT EXISTS (
		     SELECT 1 FROM transactions t2
		     WHERE t2.description = t1.description
		       AND t2.date < date('now', '-7 days')
		   )`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var alerts []SpendingAlert
	for rows.Next() {
		var desc, date string
		var amount float64
		if rows.Scan(&desc, &amount, &date) == nil {
			normalized := NormalizeMerchant(desc)
			alerts = append(alerts, SpendingAlert{
				Type:        AlertNewLargeMerchant,
				Severity:    SeverityInfo,
				Message:     fmt.Sprintf("New merchant: $%.2f at %s (first purchase)", amount, normalized),
				Amount:      math.Round(amount*100) / 100,
				Description: normalized,
				DetectedAt:  now,
			})
		}
	}
	return alerts
}

// detectSubscriptionChanges finds recurring charges where the amount has drifted.
func detectSubscriptionChanges(ctx context.Context, db *sql.DB, now string) []SpendingAlert {
	recurring := RecurringDetector(ctx, db)

	var alerts []SpendingAlert
	for _, rc := range recurring {
		// Compare last charge to the average
		var lastAmount float64
		err := db.QueryRowContext(ctx,
			`SELECT ABS(amount) FROM transactions
			 WHERE description = ? AND type = 'expense'
			 ORDER BY date DESC LIMIT 1`,
			rc.Description).Scan(&lastAmount)
		if err != nil {
			continue
		}

		if rc.Amount > 0 {
			drift := (lastAmount - rc.Amount) / rc.Amount
			if drift > 0.05 { // 5% increase
				alerts = append(alerts, SpendingAlert{
					Type:        AlertSubscriptionChange,
					Severity:    SeverityWarning,
					Message:     fmt.Sprintf("%s increased from $%.2f to $%.2f (+%.0f%%)", rc.Description, rc.Amount, lastAmount, drift*100),
					Amount:      math.Round(lastAmount*100) / 100,
					Description: rc.Description,
					DetectedAt:  now,
				})
			}
		}
	}
	return alerts
}
