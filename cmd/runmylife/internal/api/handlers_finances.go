package api

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"time"
)

func handleFinanceSummary(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		now := time.Now()
		monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")

		data := map[string]any{}

		// Month-to-date spend
		var mtdSpend float64
		db.QueryRowContext(ctx,
			`SELECT COALESCE(SUM(ABS(amount)), 0) FROM transactions
			 WHERE date >= ? AND type = 'expense'`, monthStart,
		).Scan(&mtdSpend)
		data["mtd_spend"] = mtdSpend

		// Budget status
		rows, err := db.QueryContext(ctx,
			`SELECT category, monthly_limit FROM budgets`)
		if err == nil {
			defer rows.Close()
			var budgets []map[string]any
			for rows.Next() {
				var cat string
				var limit float64
				if rows.Scan(&cat, &limit) == nil {
					var spent float64
					db.QueryRowContext(ctx,
						`SELECT COALESCE(SUM(ABS(amount)), 0) FROM transactions
						 WHERE date >= ? AND category = ? AND type = 'expense'`,
						monthStart, cat,
					).Scan(&spent)
					budgets = append(budgets, map[string]any{
						"category":  cat,
						"budget":    limit,
						"spent":     spent,
						"remaining": limit - spent,
					})
				}
			}
			data["budgets"] = budgets
		}

		// Recent transactions (last 10)
		data["recent_transactions"] = queryTransactions(ctx, db, 10)

		WriteJSON(w, http.StatusOK, data)
	}
}

func handleFinanceTransactions(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		days := 30
		if d := r.URL.Query().Get("days"); d != "" {
			if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 && parsed <= 365 {
				days = parsed
			}
		}

		limit := 50
		if l := r.URL.Query().Get("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 500 {
				limit = parsed
			}
		}

		since := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
		rows, err := db.QueryContext(ctx,
			`SELECT id, date, amount, category, description, type
			 FROM transactions WHERE date >= ?
			 ORDER BY date DESC LIMIT ?`, since, limit)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "query failed")
			return
		}
		defer rows.Close()

		var txns []map[string]any
		for rows.Next() {
			var id, date, category, txnType string
			var amount float64
			var desc sql.NullString
			if rows.Scan(&id, &date, &amount, &category, &desc, &txnType) == nil {
				t := map[string]any{
					"id": id, "date": date,
					"amount": amount, "category": category, "type": txnType,
				}
				if desc.Valid {
					t["description"] = desc.String
				}
				txns = append(txns, t)
			}
		}

		WriteJSONMeta(w, http.StatusOK, txns, map[string]any{
			"total": len(txns),
			"days":  days,
		})
	}
}

func queryTransactions(ctx context.Context, db *sql.DB, limit int) []map[string]any {
	rows, err := db.QueryContext(ctx,
		`SELECT date, amount, category, description FROM transactions
		 ORDER BY date DESC LIMIT ?`, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var txns []map[string]any
	for rows.Next() {
		var date, category string
		var amount float64
		var desc sql.NullString
		if rows.Scan(&date, &amount, &category, &desc) == nil {
			t := map[string]any{
				"date": date, "amount": amount, "category": category,
			}
			if desc.Valid {
				t["description"] = desc.String
			}
			txns = append(txns, t)
		}
	}
	return txns
}
