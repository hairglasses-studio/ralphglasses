// Package finances provides MCP tools for personal finance tracking.
package finances

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/runmylife/internal/finance"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/common"
)

// Module implements the ToolModule interface for finance management.
type Module struct{}

func (m *Module) Name() string        { return "finances" }
func (m *Module) Description() string { return "Transaction tracking, budgets, and monthly summaries" }

var financesHints = map[string]string{
	"transactions/add":       "Record a new transaction",
	"transactions/list":      "List recent transactions",
	"transactions/search":    "Search transactions by description or category",
	"budget/set":             "Set monthly budget for a category",
	"budget/check":           "Check spending vs budget for current month",
	"summary/monthly":        "View monthly income and expense summary",
	"import/csv":             "Import Rocket Money CSV export",
	"import/history":         "View past CSV imports",
	"subscriptions/list":     "List tracked subscriptions",
	"subscriptions/add":      "Add a subscription",
	"subscriptions/review":   "Review subscriptions for savings opportunities",
	"subscriptions/cancel":   "Mark subscription as cancelled",
	"paycheck/allocate":      "Calculate paycheck allocation",
	"paycheck/set_rule":      "Set an allocation rule (fixed or percent)",
	"paycheck/rules":         "View allocation rules",
	"forecast/cashflow":      "30/60/90 day cash flow forecast",
	"analytics/patterns":     "Spending patterns: weekday averages, top merchants, category trends",
	"analytics/anomalies":    "Detect spending anomalies (large transactions, high daily spend, price changes)",
	"analytics/recurring":    "Detect recurring charges and subscriptions from transaction history",
}

func (m *Module) Tools() []tools.ToolDefinition {
	dispatcher := common.NewDispatcher("finances").
		Domain("transactions", common.ActionRegistry{
			"add":    handleTransactionsAdd,
			"list":   handleTransactionsList,
			"search": handleTransactionsSearch,
		}).
		Domain("budget", common.ActionRegistry{
			"set":   handleBudgetSet,
			"check": handleBudgetCheck,
		}).
		Domain("summary", common.ActionRegistry{
			"monthly": handleSummaryMonthly,
		}).
		Domain("import", common.ActionRegistry{
			"csv":     handleImportCSV,
			"history": handleImportHistory,
		}).
		Domain("subscriptions", common.ActionRegistry{
			"list":   handleSubscriptionsList,
			"add":    handleSubscriptionsAdd,
			"review": handleSubscriptionsReview,
			"cancel": handleSubscriptionsCancel,
		}).
		Domain("paycheck", common.ActionRegistry{
			"allocate": handlePaycheckAllocate,
			"set_rule": handlePaycheckSetRule,
			"rules":    handlePaycheckRules,
		}).
		Domain("forecast", common.ActionRegistry{
			"cashflow": handleForecastCashflow,
		}).
		Domain("analytics", common.ActionRegistry{
			"patterns":  handleAnalyticsPatterns,
			"anomalies": handleAnalyticsAnomalies,
			"recurring": handleAnalyticsRecurring,
		})

	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_finances",
				mcp.WithDescription(
					"Finance management gateway.\n\n"+
						dispatcher.DescribeActionsWithHints(financesHints),
				),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Domain: transactions, budget, summary, import, subscriptions, paycheck, forecast, analytics")),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action within domain")),
				mcp.WithNumber("amount", mcp.Description("Amount")),
				mcp.WithString("category", mcp.Description("Category")),
				mcp.WithString("description", mcp.Description("Description")),
				mcp.WithString("date", mcp.Description("Date YYYY-MM-DD")),
				mcp.WithString("type", mcp.Description("Transaction type: income or expense")),
				mcp.WithString("month", mcp.Description("Month YYYY-MM")),
				mcp.WithNumber("monthly_limit", mcp.Description("Monthly budget limit")),
				mcp.WithString("query", mcp.Description("Search query")),
				mcp.WithNumber("limit", mcp.Description("Max results (default 20)")),
				// Import params
				mcp.WithString("file_path", mcp.Description("CSV file path for import")),
				mcp.WithString("source", mcp.Description("Import source: rocket_money, mint, generic")),
				// Subscription params
				mcp.WithNumber("subscription_id", mcp.Description("Subscription ID")),
				mcp.WithString("name", mcp.Description("Subscription or rule name")),
				mcp.WithString("frequency", mcp.Description("Frequency: weekly, monthly, quarterly, annual")),
				mcp.WithString("importance", mcp.Description("Importance: essential, keep, review, cancel")),
				mcp.WithString("cancel_url", mcp.Description("Cancellation URL")),
				// Paycheck params
				mcp.WithNumber("paycheck_amount", mcp.Description("Gross paycheck amount")),
				mcp.WithString("allocation_type", mcp.Description("Allocation type: fixed or percent")),
				mcp.WithNumber("priority", mcp.Description("Rule priority (lower = first)")),
				mcp.WithNumber("rule_id", mcp.Description("Allocation rule ID")),
				// Forecast params
				mcp.WithNumber("days", mcp.Description("Forecast window in days (default 90)")),
			),
			Handler:    tools.ToolHandlerFunc(dispatcher.Handler()),
			Category:   "finances",
			Tags:       []string{"finances", "budget", "money"},
			Complexity: tools.ComplexityModerate,
			IsWrite:    true,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

func handleTransactionsAdd(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	amount := common.GetFloatParam(req, "amount", 0)
	if amount == 0 {
		return common.CodedErrorResultf(common.ErrInvalidParam, "amount is required for transactions/add"), nil
	}
	category := common.GetStringParam(req, "category", "")
	description := common.GetStringParam(req, "description", "")
	date := common.GetStringParam(req, "date", time.Now().Format("2006-01-02"))
	txnType := common.GetStringParam(req, "type", "expense")

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	id := fmt.Sprintf("txn-%d", time.Now().UnixNano())
	_, err = database.SqlDB().ExecContext(ctx,
		`INSERT INTO transactions (id, amount, category, description, date, type, created_at) VALUES (?, ?, ?, ?, ?, ?, datetime('now'))`,
		id, amount, category, description, date, txnType,
	)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	return tools.TextResult(fmt.Sprintf("# Transaction Recorded\n\n- **ID:** %s\n- **Amount:** %.2f\n- **Type:** %s\n- **Category:** %s\n- **Date:** %s\n- **Description:** %s",
		id, amount, txnType, category, date, description)), nil
}

func handleTransactionsList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	limit := common.GetLimitParam(req, 20)

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	rows, err := database.SqlDB().QueryContext(ctx,
		"SELECT id, date, amount, type, category, description FROM transactions ORDER BY date DESC, created_at DESC LIMIT ?",
		limit,
	)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("Transactions")
	headers := []string{"Date", "Amount", "Type", "Category", "Description"}
	var tableRows [][]string

	for rows.Next() {
		var id, date, txnType, category, description string
		var amount float64
		if err := rows.Scan(&id, &date, &amount, &txnType, &category, &description); err != nil {
			continue
		}
		tableRows = append(tableRows, []string{date, fmt.Sprintf("%.2f", amount), txnType, category, description})
	}

	if len(tableRows) == 0 {
		md.EmptyList("transactions")
	} else {
		md.Table(headers, tableRows)
	}
	return tools.TextResult(md.String()), nil
}

func handleTransactionsSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query := common.GetStringParam(req, "query", "")
	if query == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "query is required for transactions/search"), nil
	}
	limit := common.GetLimitParam(req, 20)

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	pattern := "%" + query + "%"
	rows, err := database.SqlDB().QueryContext(ctx,
		"SELECT id, date, amount, type, category, description FROM transactions WHERE description LIKE ? OR category LIKE ? ORDER BY date DESC LIMIT ?",
		pattern, pattern, limit,
	)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Transactions: \"%s\"", query))
	headers := []string{"Date", "Amount", "Type", "Category", "Description"}
	var tableRows [][]string

	for rows.Next() {
		var id, date, txnType, category, description string
		var amount float64
		if err := rows.Scan(&id, &date, &amount, &txnType, &category, &description); err != nil {
			continue
		}
		tableRows = append(tableRows, []string{date, fmt.Sprintf("%.2f", amount), txnType, category, description})
	}

	if len(tableRows) == 0 {
		md.EmptyList("matching transactions")
	} else {
		md.Table(headers, tableRows)
	}
	return tools.TextResult(md.String()), nil
}

func handleBudgetSet(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	category := common.GetStringParam(req, "category", "")
	if category == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "category is required for budget/set"), nil
	}
	monthlyLimit := common.GetFloatParam(req, "monthly_limit", 0)
	if monthlyLimit == 0 {
		return common.CodedErrorResultf(common.ErrInvalidParam, "monthly_limit is required for budget/set"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	_, err = database.SqlDB().ExecContext(ctx,
		`INSERT OR REPLACE INTO budgets (category, monthly_limit, updated_at) VALUES (?, ?, datetime('now'))`,
		category, monthlyLimit,
	)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	return tools.TextResult(fmt.Sprintf("# Budget Set\n\n- **Category:** %s\n- **Monthly Limit:** %.2f",
		category, monthlyLimit)), nil
}

func handleBudgetCheck(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	month := common.GetStringParam(req, "month", time.Now().Format("2006-01"))

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	rows, err := database.SqlDB().QueryContext(ctx,
		`SELECT b.category, b.monthly_limit, COALESCE(SUM(t.amount), 0) as spent
		FROM budgets b
		LEFT JOIN transactions t ON t.category = b.category
			AND t.type = 'expense'
			AND strftime('%Y-%m', t.date) = ?
		GROUP BY b.category, b.monthly_limit
		ORDER BY b.category`,
		month,
	)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Budget Check — %s", month))
	headers := []string{"Category", "Budget", "Spent", "Remaining", "Status"}
	var tableRows [][]string

	for rows.Next() {
		var category string
		var monthlyLimit, spent float64
		if err := rows.Scan(&category, &monthlyLimit, &spent); err != nil {
			continue
		}
		remaining := monthlyLimit - spent
		status := "OK"
		if remaining < 0 {
			status = "OVER"
		} else if remaining < monthlyLimit*0.1 {
			status = "LOW"
		}
		tableRows = append(tableRows, []string{
			category,
			fmt.Sprintf("%.2f", monthlyLimit),
			fmt.Sprintf("%.2f", spent),
			fmt.Sprintf("%.2f", remaining),
			status,
		})
	}

	if len(tableRows) == 0 {
		md.EmptyList("budgets")
	} else {
		md.Table(headers, tableRows)
	}
	return tools.TextResult(md.String()), nil
}

func handleSummaryMonthly(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	month := common.GetStringParam(req, "month", time.Now().Format("2006-01"))

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	// Get totals by type
	var totalIncome, totalExpense float64
	err = database.SqlDB().QueryRowContext(ctx,
		"SELECT COALESCE(SUM(amount), 0) FROM transactions WHERE type = 'income' AND strftime('%Y-%m', date) = ?",
		month,
	).Scan(&totalIncome)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	err = database.SqlDB().QueryRowContext(ctx,
		"SELECT COALESCE(SUM(amount), 0) FROM transactions WHERE type = 'expense' AND strftime('%Y-%m', date) = ?",
		month,
	).Scan(&totalExpense)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Monthly Summary — %s", month))
	md.KeyValue("Total Income", fmt.Sprintf("%.2f", totalIncome))
	md.KeyValue("Total Expenses", fmt.Sprintf("%.2f", totalExpense))
	md.KeyValue("Net", fmt.Sprintf("%.2f", totalIncome-totalExpense))
	md.Text("")

	// Breakdown by category for expenses
	rows, err := database.SqlDB().QueryContext(ctx,
		"SELECT category, SUM(amount) as total FROM transactions WHERE type = 'expense' AND strftime('%Y-%m', date) = ? GROUP BY category ORDER BY total DESC",
		month,
	)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md.Section("Expense Breakdown")
	headers := []string{"Category", "Amount"}
	var tableRows [][]string

	for rows.Next() {
		var category string
		var total float64
		if err := rows.Scan(&category, &total); err != nil {
			continue
		}
		if category == "" {
			category = "(uncategorized)"
		}
		tableRows = append(tableRows, []string{category, fmt.Sprintf("%.2f", total)})
	}

	if len(tableRows) == 0 {
		md.EmptyList("expenses")
	} else {
		md.Table(headers, tableRows)
	}

	// Breakdown by category for income
	incRows, err := database.SqlDB().QueryContext(ctx,
		"SELECT category, SUM(amount) as total FROM transactions WHERE type = 'income' AND strftime('%Y-%m', date) = ? GROUP BY category ORDER BY total DESC",
		month,
	)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer incRows.Close()

	md.Section("Income Breakdown")
	var incTableRows [][]string

	for incRows.Next() {
		var category string
		var total float64
		if err := incRows.Scan(&category, &total); err != nil {
			continue
		}
		if category == "" {
			category = "(uncategorized)"
		}
		incTableRows = append(incTableRows, []string{category, fmt.Sprintf("%.2f", total)})
	}

	if len(incTableRows) == 0 {
		md.EmptyList("income")
	} else {
		md.Table(headers, incTableRows)
	}

	return tools.TextResult(md.String()), nil
}

// --- Import Handlers ---

func handleImportCSV(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	filePath := common.GetStringParam(req, "file_path", "")
	if filePath == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "file_path is required"), nil
	}
	source := common.GetStringParam(req, "source", "rocket_money")

	f, err := os.Open(filePath)
	if err != nil {
		return common.CodedErrorResultf(common.ErrInvalidParam, "cannot open file: %v", err), nil
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true

	// Read header
	header, err := reader.Read()
	if err != nil {
		return common.CodedErrorResultf(common.ErrInvalidParam, "cannot read CSV header: %v", err), nil
	}

	// Map column indices
	colMap := make(map[string]int)
	for i, h := range header {
		colMap[strings.ToLower(strings.TrimSpace(h))] = i
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	imported, skipped := 0, 0

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			skipped++
			continue
		}

		var date, description, category, txnType string
		var amount float64

		switch source {
		case "rocket_money":
			// Rocket Money exports: Date, Description, Category, Amount, Account, Notes
			date = getCSVField(record, colMap, "date")
			description = getCSVField(record, colMap, "description")
			category = getCSVField(record, colMap, "category")
			amtStr := getCSVField(record, colMap, "amount")
			amount, _ = strconv.ParseFloat(strings.ReplaceAll(strings.TrimPrefix(amtStr, "$"), ",", ""), 64)
			if amount > 0 {
				txnType = "income"
			} else {
				txnType = "expense"
				amount = -amount
			}
		case "mint":
			date = getCSVField(record, colMap, "date")
			description = getCSVField(record, colMap, "description")
			category = getCSVField(record, colMap, "category")
			amtStr := getCSVField(record, colMap, "amount")
			amount, _ = strconv.ParseFloat(strings.ReplaceAll(amtStr, ",", ""), 64)
			txnType = strings.ToLower(getCSVField(record, colMap, "transaction type"))
			if txnType == "" {
				txnType = "expense"
			}
		default:
			// Generic: expects date, description, amount, category columns
			date = getCSVField(record, colMap, "date")
			description = getCSVField(record, colMap, "description")
			category = getCSVField(record, colMap, "category")
			amtStr := getCSVField(record, colMap, "amount")
			amount, _ = strconv.ParseFloat(strings.ReplaceAll(strings.ReplaceAll(amtStr, "$", ""), ",", ""), 64)
			txnType = getCSVField(record, colMap, "type")
			if txnType == "" {
				txnType = "expense"
			}
		}

		if date == "" || amount == 0 {
			skipped++
			continue
		}

		// Normalize date formats
		for _, layout := range []string{"2006-01-02", "01/02/2006", "1/2/2006", "Jan 2, 2006"} {
			if t, err := time.Parse(layout, date); err == nil {
				date = t.Format("2006-01-02")
				break
			}
		}

		id := fmt.Sprintf("csv-%s-%d", source, time.Now().UnixNano()+int64(imported))
		_, err = db.ExecContext(ctx,
			"INSERT OR IGNORE INTO transactions (id, amount, category, description, date, type) VALUES (?, ?, ?, ?, ?, ?)",
			id, amount, category, description, date, txnType)
		if err != nil {
			skipped++
			continue
		}
		imported++
	}

	// Record import
	db.ExecContext(ctx,
		"INSERT INTO csv_imports (source, filename, rows_imported, rows_skipped) VALUES (?, ?, ?, ?)",
		source, filePath, imported, skipped)

	md := common.NewMarkdownBuilder().Title("CSV Import Complete")
	md.KeyValue("Source", source)
	md.KeyValue("Imported", fmt.Sprintf("%d transactions", imported))
	md.KeyValue("Skipped", fmt.Sprintf("%d rows", skipped))
	return tools.TextResult(md.String()), nil
}

func handleImportHistory(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	rows, err := db.QueryContext(ctx,
		"SELECT source, filename, rows_imported, rows_skipped, imported_at FROM csv_imports ORDER BY imported_at DESC LIMIT 10")
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("Import History")
	var tableRows [][]string
	for rows.Next() {
		var source, filename, importedAt string
		var imported, skipped int
		if rows.Scan(&source, &filename, &imported, &skipped, &importedAt) == nil {
			tableRows = append(tableRows, []string{importedAt[:10], source, fmt.Sprintf("%d", imported), fmt.Sprintf("%d", skipped)})
		}
	}
	if len(tableRows) == 0 {
		md.EmptyList("imports")
	} else {
		md.Table([]string{"Date", "Source", "Imported", "Skipped"}, tableRows)
	}
	return tools.TextResult(md.String()), nil
}

// --- Subscription Handlers ---

func handleSubscriptionsList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	rows, err := db.QueryContext(ctx, `
		SELECT id, name, amount, frequency, category, importance, next_charge_date, active
		FROM subscriptions ORDER BY
			CASE importance WHEN 'cancel' THEN 0 WHEN 'review' THEN 1 WHEN 'keep' THEN 2 ELSE 3 END,
			amount DESC`)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("Subscriptions")
	var totalMonthly float64
	var tableRows [][]string
	for rows.Next() {
		var id, active int
		var name, freq, cat, importance string
		var amount float64
		var nextCharge *string
		if rows.Scan(&id, &name, &amount, &freq, &cat, &importance, &nextCharge, &active) == nil {
			status := importance
			if active == 0 {
				status = "cancelled"
			}
			monthly := toMonthly(amount, freq)
			if active == 1 {
				totalMonthly += monthly
			}
			next := "-"
			if nextCharge != nil {
				next = *nextCharge
			}
			tableRows = append(tableRows, []string{
				fmt.Sprintf("%d", id), name, fmt.Sprintf("$%.2f", amount), freq,
				cat, status, next, fmt.Sprintf("$%.2f", monthly),
			})
		}
	}
	if len(tableRows) == 0 {
		md.EmptyList("subscriptions")
	} else {
		md.Table([]string{"ID", "Name", "Amount", "Freq", "Category", "Status", "Next", "Monthly"}, tableRows)
		md.KeyValue("Total monthly cost", fmt.Sprintf("$%.2f", totalMonthly))
		md.KeyValue("Annual cost", fmt.Sprintf("$%.2f", totalMonthly*12))
	}
	return tools.TextResult(md.String()), nil
}

func handleSubscriptionsAdd(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := common.GetStringParam(req, "name", "")
	if name == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "name is required"), nil
	}
	amount := common.GetFloatParam(req, "amount", 0)
	if amount == 0 {
		return common.CodedErrorResultf(common.ErrInvalidParam, "amount is required"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	freq := common.GetStringParam(req, "frequency", "monthly")
	cat := common.GetStringParam(req, "category", "")
	importance := common.GetStringParam(req, "importance", "keep")
	cancelURL := common.GetStringParam(req, "cancel_url", "")

	result, err := db.ExecContext(ctx,
		`INSERT INTO subscriptions (name, amount, frequency, category, importance, cancel_url)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		name, amount, freq, cat, importance, cancelURL)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	id, _ := result.LastInsertId()
	monthly := toMonthly(amount, freq)
	return tools.TextResult(fmt.Sprintf("Subscription #%d added: %s ($%.2f/%s = $%.2f/mo).", id, name, amount, freq, monthly)), nil
}

func handleSubscriptionsReview(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	md := common.NewMarkdownBuilder().Title("Subscription Review")

	// Flagged for review or cancel
	rows, err := db.QueryContext(ctx, `
		SELECT id, name, amount, frequency, category, importance, cancel_url
		FROM subscriptions WHERE active = 1 AND importance IN ('review', 'cancel')
		ORDER BY amount DESC`)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	var savings float64
	var tableRows [][]string
	for rows.Next() {
		var id int
		var name, freq, cat, importance, cancelURL string
		var amount float64
		if rows.Scan(&id, &name, &amount, &freq, &cat, &importance, &cancelURL) == nil {
			monthly := toMonthly(amount, freq)
			savings += monthly
			urlNote := ""
			if cancelURL != "" {
				urlNote = " (has cancel URL)"
			}
			tableRows = append(tableRows, []string{
				fmt.Sprintf("%d", id), name, fmt.Sprintf("$%.2f", monthly), importance, cat + urlNote,
			})
		}
	}

	if len(tableRows) == 0 {
		md.Text("No subscriptions flagged for review. Use importance='review' or 'cancel' to flag them.")
	} else {
		md.Table([]string{"ID", "Name", "Monthly", "Status", "Category"}, tableRows)
		md.KeyValue("Potential monthly savings", fmt.Sprintf("$%.2f", savings))
		md.KeyValue("Potential annual savings", fmt.Sprintf("$%.2f", savings*12))
	}

	return tools.TextResult(md.String()), nil
}

func handleSubscriptionsCancel(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	subID := int64(common.GetIntParam(req, "subscription_id", 0))
	if subID == 0 {
		return common.CodedErrorResultf(common.ErrInvalidParam, "subscription_id is required"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	db.ExecContext(ctx, "UPDATE subscriptions SET active = 0, updated_at = datetime('now') WHERE id = ?", subID)
	return tools.TextResult(fmt.Sprintf("Subscription #%d cancelled.", subID)), nil
}

// --- Paycheck Handlers ---

func handlePaycheckAllocate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	paycheckAmount := common.GetFloatParam(req, "paycheck_amount", 0)
	if paycheckAmount == 0 {
		return common.CodedErrorResultf(common.ErrInvalidParam, "paycheck_amount is required"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	rows, err := db.QueryContext(ctx,
		"SELECT id, name, allocation_type, amount, category FROM paycheck_allocations WHERE active = 1 ORDER BY priority ASC")
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Paycheck Allocation — $%.2f", paycheckAmount))

	remaining := paycheckAmount
	var tableRows [][]string
	for rows.Next() {
		var id int
		var name, allocType, category string
		var ruleAmount float64
		if rows.Scan(&id, &name, &allocType, &ruleAmount, &category) != nil {
			continue
		}

		var allocated float64
		switch allocType {
		case "fixed":
			allocated = ruleAmount
		case "percent":
			allocated = paycheckAmount * ruleAmount / 100
		}

		if allocated > remaining {
			allocated = remaining
		}
		remaining -= allocated

		tableRows = append(tableRows, []string{
			name, allocType, fmt.Sprintf("$%.2f", ruleAmount),
			fmt.Sprintf("$%.2f", allocated), category,
		})
	}

	if len(tableRows) == 0 {
		md.Text("No allocation rules set. Use `paycheck/set_rule` to create them.")
	} else {
		md.Table([]string{"Name", "Type", "Rule", "Allocated", "Category"}, tableRows)
	}
	md.KeyValue("Remaining (discretionary)", fmt.Sprintf("$%.2f", remaining))

	return tools.TextResult(md.String()), nil
}

func handlePaycheckSetRule(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := common.GetStringParam(req, "name", "")
	if name == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "name is required"), nil
	}
	amount := common.GetFloatParam(req, "amount", 0)
	if amount == 0 {
		return common.CodedErrorResultf(common.ErrInvalidParam, "amount is required"), nil
	}
	allocType := common.GetStringParam(req, "allocation_type", "fixed")
	category := common.GetStringParam(req, "category", "")
	priority := common.GetIntParam(req, "priority", 5)

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	result, err := db.ExecContext(ctx,
		"INSERT INTO paycheck_allocations (name, allocation_type, amount, category, priority) VALUES (?, ?, ?, ?, ?)",
		name, allocType, amount, category, priority)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	id, _ := result.LastInsertId()

	desc := fmt.Sprintf("$%.2f", amount)
	if allocType == "percent" {
		desc = fmt.Sprintf("%.1f%%", amount)
	}
	return tools.TextResult(fmt.Sprintf("Allocation rule #%d: %s → %s (%s, priority %d).", id, name, desc, allocType, priority)), nil
}

func handlePaycheckRules(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	rows, err := db.QueryContext(ctx,
		"SELECT id, name, allocation_type, amount, category, priority FROM paycheck_allocations WHERE active = 1 ORDER BY priority ASC")
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("Paycheck Allocation Rules")
	var tableRows [][]string
	for rows.Next() {
		var id, priority int
		var name, allocType, category string
		var amount float64
		if rows.Scan(&id, &name, &allocType, &amount, &category, &priority) == nil {
			desc := fmt.Sprintf("$%.2f", amount)
			if allocType == "percent" {
				desc = fmt.Sprintf("%.1f%%", amount)
			}
			tableRows = append(tableRows, []string{
				fmt.Sprintf("%d", id), fmt.Sprintf("%d", priority), name, allocType, desc, category,
			})
		}
	}
	if len(tableRows) == 0 {
		md.EmptyList("allocation rules")
	} else {
		md.Table([]string{"ID", "Priority", "Name", "Type", "Amount", "Category"}, tableRows)
	}
	return tools.TextResult(md.String()), nil
}

// --- Forecast Handler ---

func handleForecastCashflow(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	days := common.GetIntParam(req, "days", 90)
	now := time.Now()

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Cash Flow Forecast — %d Days", days))

	// Calculate average daily spending from last 90 days
	past90 := now.AddDate(0, 0, -90).Format("2006-01-02")
	var totalExpense90, totalIncome90 float64
	db.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(amount), 0) FROM transactions WHERE type = 'expense' AND date >= ?", past90).Scan(&totalExpense90)
	db.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(amount), 0) FROM transactions WHERE type = 'income' AND date >= ?", past90).Scan(&totalIncome90)

	avgDailyExpense := totalExpense90 / 90
	avgDailyIncome := totalIncome90 / 90

	md.Section("Historical Averages (90d)")
	md.KeyValue("Avg daily expense", fmt.Sprintf("$%.2f", avgDailyExpense))
	md.KeyValue("Avg daily income", fmt.Sprintf("$%.2f", avgDailyIncome))
	md.KeyValue("Avg daily net", fmt.Sprintf("$%.2f", avgDailyIncome-avgDailyExpense))

	// Upcoming subscription charges
	var monthlySubCost float64
	db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(
			CASE frequency
				WHEN 'weekly' THEN amount * 4.33
				WHEN 'monthly' THEN amount
				WHEN 'quarterly' THEN amount / 3
				WHEN 'annual' THEN amount / 12
			END
		), 0) FROM subscriptions WHERE active = 1`).Scan(&monthlySubCost)

	// Forecast at 30/60/90 day marks
	md.Section("Projected Spending")
	var forecastTable [][]string
	for _, window := range []int{30, 60, 90} {
		if window > days {
			break
		}
		projExpense := avgDailyExpense * float64(window)
		projIncome := avgDailyIncome * float64(window)
		subCost := monthlySubCost * float64(window) / 30
		forecastTable = append(forecastTable, []string{
			fmt.Sprintf("%d days", window),
			fmt.Sprintf("$%.0f", projIncome),
			fmt.Sprintf("$%.0f", projExpense),
			fmt.Sprintf("$%.0f", subCost),
			fmt.Sprintf("$%.0f", projIncome-projExpense),
		})
	}
	md.Table([]string{"Window", "Proj. Income", "Proj. Expense", "Subscriptions", "Net"}, forecastTable)

	// House bills upcoming (from ArtHouse)
	var houseBillTotal float64
	db.QueryRowContext(ctx, "SELECT COALESCE(SUM(amount), 0) FROM house_bills").Scan(&houseBillTotal)
	if houseBillTotal > 0 {
		md.Section("House Bills")
		md.KeyValue("Monthly house bills", fmt.Sprintf("$%.2f", houseBillTotal))
		md.KeyValue("Your share (1/5)", fmt.Sprintf("$%.2f", houseBillTotal/5))
	}

	// Budget status
	md.Section("Monthly Subscription Burn")
	md.KeyValue("Active subscriptions", fmt.Sprintf("$%.2f/mo", monthlySubCost))
	md.KeyValue("Annual", fmt.Sprintf("$%.2f", monthlySubCost*12))

	return tools.TextResult(md.String()), nil
}

// --- Helpers ---

func getCSVField(record []string, colMap map[string]int, field string) string {
	if idx, ok := colMap[field]; ok && idx < len(record) {
		return strings.TrimSpace(record[idx])
	}
	return ""
}

func toMonthly(amount float64, frequency string) float64 {
	switch frequency {
	case "weekly":
		return amount * 4.33
	case "monthly":
		return amount
	case "quarterly":
		return amount / 3
	case "annual":
		return amount / 12
	default:
		return amount
	}
}

// --- Analytics Handlers ---

func handleAnalyticsPatterns(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	sqlDB := database.SqlDB()

	category := common.GetStringParam(req, "category", "")
	days := int(common.GetFloatParam(req, "days", 30))

	var sb strings.Builder
	sb.WriteString("## Spending Patterns\n\n")

	// Weekday averages
	weekday := finance.WeekdayPattern(ctx, sqlDB)
	if len(weekday) > 0 {
		sb.WriteString("### Average Spend by Day\n")
		for _, day := range []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"} {
			if avg, ok := weekday[day]; ok {
				sb.WriteString(fmt.Sprintf("- **%s**: $%.2f\n", day, avg))
			}
		}
		sb.WriteString("\n")
	}

	// Top merchants
	top := finance.TopMerchants(ctx, sqlDB, days)
	if len(top) > 0 {
		sb.WriteString(fmt.Sprintf("### Top Merchants (%d days)\n", days))
		for i, m := range top {
			sb.WriteString(fmt.Sprintf("%d. **%s** — $%.2f (%d transactions)\n", i+1, m.Description, m.Total, m.Count))
		}
		sb.WriteString("\n")
	}

	// Category trend if specified
	if category != "" {
		trend := finance.CategoryTrend(ctx, sqlDB, category, 6)
		if len(trend) > 0 {
			sb.WriteString(fmt.Sprintf("### %s — 6 Month Trend\n", category))
			for _, t := range trend {
				sb.WriteString(fmt.Sprintf("- %s: $%.2f\n", t.Month, t.Amount))
			}
		}
	}

	return tools.TextResult(sb.String()), nil
}

func handleAnalyticsAnomalies(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	sqlDB := database.SqlDB()

	alerts := finance.DetectAnomalies(ctx, sqlDB)
	if len(alerts) == 0 {
		return tools.TextResult("No spending anomalies detected. Everything looks normal."), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Spending Alerts (%d found)\n\n", len(alerts)))
	for _, a := range alerts {
		icon := "ℹ️"
		switch a.Severity {
		case finance.SeverityWarning:
			icon = "⚠️"
		case finance.SeverityAlert:
			icon = "🚨"
		}
		sb.WriteString(fmt.Sprintf("%s **%s**: %s\n\n", icon, a.Type, a.Message))
	}

	return tools.TextResult(sb.String()), nil
}

func handleAnalyticsRecurring(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	sqlDB := database.SqlDB()

	recurring := finance.RecurringDetector(ctx, sqlDB)
	if len(recurring) == 0 {
		return tools.TextResult("No recurring charges detected in the last 90 days."), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Recurring Charges (%d detected)\n\n", len(recurring)))
	var totalMonthly float64
	for _, r := range recurring {
		sb.WriteString(fmt.Sprintf("- **%s** — $%.2f/avg", r.Description, r.Amount))
		if r.Category != "" {
			sb.WriteString(fmt.Sprintf(" [%s]", r.Category))
		}
		sb.WriteString(fmt.Sprintf(" (%d charges, last: %s)\n", r.Occurrences, r.LastSeen))
		totalMonthly += r.Amount
	}
	sb.WriteString(fmt.Sprintf("\n**Estimated monthly recurring**: $%.2f\n", totalMonthly))

	return tools.TextResult(sb.String()), nil
}
