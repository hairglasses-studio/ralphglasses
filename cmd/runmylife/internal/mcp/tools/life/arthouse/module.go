// Package arthouse provides the ArtHouse life category MCP tool.
// Covers roommate coordination: groceries, bills, chores, house management, and budget.
package arthouse

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/common"
)

type Module struct{}

func (m *Module) Name() string        { return "arthouse" }
func (m *Module) Description() string { return "ArtHouse roommate coordination: groceries, bills, chores, house" }

var arthouseHints = map[string]string{
	"grocery/list":          "Current grocery list",
	"grocery/add":           "Add items to grocery list",
	"grocery/claim":         "Claim items for shopping",
	"grocery/trip_plan":     "Plan next grocery/Costco run",
	"grocery/trip_complete": "Complete trip with actual cost and splits",
	"grocery/price_track":   "Track Costco item prices",
	"bills/list":            "List house bills",
	"bills/add":             "Add a recurring bill",
	"bills/split":           "Calculate who owes what this period",
	"bills/settle":          "Record a payment/settlement",
	"bills/history":         "Payment history",
	"chores/list":           "Current chore assignments",
	"chores/rotate":         "Advance chore rotation",
	"chores/complete":       "Mark chore as done",
	"chores/add":            "Add a new chore definition",
	"house/members":         "Household members and karma",
	"house/announcements":   "Announcement board",
	"house/maintenance":     "Maintenance requests",
	"house/dashboard":       "Full house dashboard",
	"budget/overview":       "House budget overview",
	"budget/savings_goals":  "Savings goals tracking",
}

func (m *Module) Tools() []tools.ToolDefinition {
	dispatcher := common.NewDispatcher("arthouse").
		Domain("grocery", common.ActionRegistry{
			"list":          handleGroceryList,
			"add":           handleGroceryAdd,
			"claim":         handleGroceryClaim,
			"trip_plan":     handleGroceryTripPlan,
			"trip_complete": handleGroceryTripComplete,
			"price_track":   handleGroceryPriceTrack,
		}).
		Domain("bills", common.ActionRegistry{
			"list":    handleBillsList,
			"add":     handleBillsAdd,
			"split":   handleBillsSplit,
			"settle":  handleBillsSettle,
			"history": handleBillsHistory,
		}).
		Domain("chores", common.ActionRegistry{
			"list":     handleChoresList,
			"rotate":   handleChoresRotate,
			"complete": handleChoresComplete,
			"add":      handleChoresAdd,
		}).
		Domain("house", common.ActionRegistry{
			"members":       handleHouseMembers,
			"announcements": handleHouseAnnouncements,
			"maintenance":   handleHouseMaintenance,
			"dashboard":     handleHouseDashboard,
		}).
		Domain("budget", common.ActionRegistry{
			"overview":      handleBudgetOverview,
			"savings_goals": handleBudgetSavingsGoals,
		})

	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_arthouse",
				mcp.WithDescription("ArtHouse roommate coordination gateway.\n\n"+
					dispatcher.DescribeActionsWithHints(arthouseHints)),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Domain: grocery, bills, chores, house, budget")),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action within domain")),
				// Grocery params
				mcp.WithString("items", mcp.Description("Grocery items (comma-separated or JSON array)")),
				mcp.WithString("category", mcp.Description("Item category or bill category")),
				mcp.WithBoolean("is_costco", mcp.Description("Costco bulk item flag")),
				mcp.WithNumber("estimated_price", mcp.Description("Estimated item price")),
				mcp.WithNumber("trip_id", mcp.Description("Grocery trip ID")),
				mcp.WithNumber("total_cost", mcp.Description("Actual total cost for trip")),
				mcp.WithString("item_name", mcp.Description("Item name for price tracking")),
				mcp.WithNumber("price", mcp.Description("Price for tracking")),
				// Bill params
				mcp.WithString("name", mcp.Description("Bill name, chore name, announcement title, etc.")),
				mcp.WithNumber("amount", mcp.Description("Bill amount or payment amount")),
				mcp.WithNumber("due_day", mcp.Description("Day of month bill is due")),
				mcp.WithString("frequency", mcp.Description("Bill frequency: monthly, quarterly, annual")),
				mcp.WithString("split_type", mcp.Description("Split type: equal, custom, single")),
				mcp.WithString("venmo_ref", mcp.Description("Venmo reference for settlement")),
				mcp.WithString("period", mcp.Description("Billing period YYYY-MM")),
				// Chore params
				mcp.WithNumber("chore_id", mcp.Description("Chore definition ID")),
				mcp.WithNumber("assignment_id", mcp.Description("Chore assignment ID")),
				mcp.WithNumber("karma_points", mcp.Description("Karma points for a chore")),
				// House params
				mcp.WithString("member_id", mcp.Description("Household member ID")),
				mcp.WithString("body", mcp.Description("Announcement body or description")),
				mcp.WithString("priority", mcp.Description("Priority: low, normal, urgent")),
				mcp.WithString("title", mcp.Description("Announcement or maintenance title")),
				mcp.WithNumber("maintenance_id", mcp.Description("Maintenance request ID")),
				mcp.WithString("status", mcp.Description("Status update")),
				// Budget params
				mcp.WithNumber("target_amount", mcp.Description("Savings goal target")),
				mcp.WithNumber("goal_id", mcp.Description("Savings goal ID")),
				mcp.WithNumber("deposit", mcp.Description("Amount to add to savings goal")),
			),
			Handler:     tools.ToolHandlerFunc(dispatcher.Handler()),
			Category:    "life",
			Subcategory: "arthouse",
			Tags:        []string{"arthouse", "roommates", "bills", "groceries", "chores"},
			Complexity:  tools.ComplexityComplex,
			IsWrite:     true,
			Timeout:     30 * time.Second,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

func getDB(ctx context.Context) (*sql.DB, error) {
	database, err := common.OpenDB()
	if err != nil {
		return nil, err
	}
	return database.SqlDB(), nil
}

func memberName(ctx context.Context, db *sql.DB, memberID string) string {
	var name string
	db.QueryRowContext(ctx, "SELECT name FROM household_members WHERE id = ?", memberID).Scan(&name)
	if name == "" {
		return memberID
	}
	return name
}

func defaultMember() string { return "member-mitch" }

func getMemberID(req mcp.CallToolRequest) string {
	id := common.GetStringParam(req, "member_id", "")
	if id == "" {
		return defaultMember()
	}
	return id
}

// --- Grocery Handlers ---

func handleGroceryList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	rows, err := db.QueryContext(ctx, `
		SELECT gi.id, gi.name, gi.quantity, gi.category, gi.is_costco, gi.estimated_price, gi.status, gi.claimed_by,
		       hm.name AS requested_by_name
		FROM grocery_items gi
		JOIN household_members hm ON hm.id = gi.requested_by
		WHERE gi.status IN ('pending', 'claimed')
		ORDER BY gi.is_costco DESC, gi.category, gi.name
	`)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("Grocery List")
	headers := []string{"ID", "Item", "Qty", "Category", "Costco", "Est. $", "Status", "By"}
	var tableRows [][]string
	var totalEst float64

	for rows.Next() {
		var id int
		var name, qty, category, status string
		var isCostco int
		var estPrice float64
		var requestedBy string
		var claimedBy sql.NullString
		if rows.Scan(&id, &name, &qty, &category, &isCostco, &estPrice, &status, &claimedBy, &requestedBy) != nil {
			continue
		}
		costcoFlag := ""
		if isCostco == 1 {
			costcoFlag = "Y"
		}
		by := requestedBy
		if claimedBy.Valid && claimedBy.String != "" {
			by = memberName(ctx, db, claimedBy.String) + " (claimed)"
		}
		totalEst += estPrice
		tableRows = append(tableRows, []string{
			fmt.Sprintf("%d", id), name, qty, category, costcoFlag,
			fmt.Sprintf("$%.2f", estPrice), status, by,
		})
	}

	if len(tableRows) == 0 {
		md.EmptyList("grocery items")
	} else {
		md.Table(headers, tableRows)
		md.KeyValue("Estimated total", fmt.Sprintf("$%.2f", totalEst))
	}

	return tools.TextResult(md.String()), nil
}

func handleGroceryAdd(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	items := common.GetStringParam(req, "items", "")
	if items == "" {
		items = common.GetStringParam(req, "name", "")
	}
	if items == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "items or name is required"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	memberID := getMemberID(req)
	isCostco := common.GetBoolParam(req, "is_costco", false)
	category := common.GetStringParam(req, "category", "general")
	estPrice := common.GetFloatParam(req, "estimated_price", 0)

	// Split comma-separated items
	itemList := strings.Split(items, ",")
	added := 0
	for _, item := range itemList {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		costcoInt := 0
		if isCostco {
			costcoInt = 1
		}
		_, err := db.ExecContext(ctx,
			"INSERT INTO grocery_items (name, category, requested_by, is_costco, estimated_price) VALUES (?, ?, ?, ?, ?)",
			item, category, memberID, costcoInt, estPrice)
		if err == nil {
			added++
		}
	}

	return tools.TextResult(fmt.Sprintf("Added %d item(s) to grocery list.", added)), nil
}

func handleGroceryClaim(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	memberID := getMemberID(req)

	// Claim all pending items (or specific trip)
	tripID := int64(common.GetIntParam(req, "trip_id", 0))
	var result sql.Result
	if tripID > 0 {
		result, err = db.ExecContext(ctx,
			"UPDATE grocery_items SET status = 'claimed', claimed_by = ? WHERE trip_id = ? AND status = 'pending'",
			memberID, tripID)
	} else {
		result, err = db.ExecContext(ctx,
			"UPDATE grocery_items SET status = 'claimed', claimed_by = ? WHERE status = 'pending'",
			memberID)
	}
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	affected, _ := result.RowsAffected()
	name := memberName(ctx, db, memberID)
	return tools.TextResult(fmt.Sprintf("%s claimed %d grocery items.", name, affected)), nil
}

func handleGroceryTripPlan(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	// Create a new trip and assign pending items to it
	plannedDate := time.Now().AddDate(0, 0, 1).Format("2006-01-02") // default: tomorrow
	result, err := db.ExecContext(ctx,
		"INSERT INTO grocery_trips (store, planned_date) VALUES ('Costco', ?)", plannedDate)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	tripID, _ := result.LastInsertId()

	// Assign pending Costco items to this trip
	updateResult, _ := db.ExecContext(ctx,
		"UPDATE grocery_items SET trip_id = ? WHERE status = 'pending' AND is_costco = 1", tripID)
	assigned, _ := updateResult.RowsAffected()

	// Also assign non-Costco pending items
	updateResult2, _ := db.ExecContext(ctx,
		"UPDATE grocery_items SET trip_id = ? WHERE status = 'pending' AND trip_id IS NULL", tripID)
	assigned2, _ := updateResult2.RowsAffected()

	// Calculate estimated total
	var estTotal float64
	db.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(estimated_price), 0) FROM grocery_items WHERE trip_id = ?", tripID).Scan(&estTotal)

	// Per-member cost estimate
	var memberCount int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM household_members WHERE is_active = 1").Scan(&memberCount)
	perPerson := 0.0
	if memberCount > 0 {
		perPerson = estTotal / float64(memberCount)
	}

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Trip #%d Planned", tripID))
	md.KeyValue("Store", "Costco")
	md.KeyValue("Planned date", plannedDate)
	md.KeyValue("Items assigned", fmt.Sprintf("%d", assigned+assigned2))
	md.KeyValue("Estimated total", fmt.Sprintf("$%.2f", estTotal))
	md.KeyValue("Per person (~%d)", fmt.Sprintf("$%.2f", perPerson))

	return tools.TextResult(md.String()), nil
}

func handleGroceryTripComplete(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	tripID := int64(common.GetIntParam(req, "trip_id", 0))
	if tripID == 0 {
		return common.CodedErrorResultf(common.ErrInvalidParam, "trip_id is required"), nil
	}
	totalCost := common.GetFloatParam(req, "total_cost", 0)
	if totalCost <= 0 {
		return common.CodedErrorResultf(common.ErrInvalidParam, "total_cost must be positive"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	memberID := getMemberID(req)

	// Update trip
	db.ExecContext(ctx,
		"UPDATE grocery_trips SET status = 'completed', total_cost = ?, shopper_id = ?, completed_at = datetime('now') WHERE id = ?",
		totalCost, memberID, tripID)

	// Mark items as purchased
	db.ExecContext(ctx,
		"UPDATE grocery_items SET status = 'purchased' WHERE trip_id = ?", tripID)

	// Calculate splits: equal among all active members
	var members []string
	rows, err := db.QueryContext(ctx, "SELECT id FROM household_members WHERE is_active = 1")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				members = append(members, id)
			}
		}
	}

	if len(members) > 0 {
		splitAmount := totalCost / float64(len(members))
		for _, mid := range members {
			paid := 0
			if mid == memberID {
				paid = 1 // shopper already paid
			}
			db.ExecContext(ctx,
				"INSERT INTO grocery_splits (trip_id, member_id, amount, paid) VALUES (?, ?, ?, ?)",
				tripID, mid, splitAmount, paid)
		}
	}

	// Award karma to shopper
	db.ExecContext(ctx,
		"UPDATE household_members SET karma_score = karma_score + 10 WHERE id = ?", memberID)

	shopperName := memberName(ctx, db, memberID)
	perPerson := totalCost / math.Max(float64(len(members)), 1)

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Trip #%d Complete", tripID))
	md.KeyValue("Shopper", shopperName+" (+10 karma)")
	md.KeyValue("Total cost", fmt.Sprintf("$%.2f", totalCost))
	md.KeyValue("Per person", fmt.Sprintf("$%.2f", perPerson))
	md.KeyValue("Members", fmt.Sprintf("%d", len(members)))

	return tools.TextResult(md.String()), nil
}

func handleGroceryPriceTrack(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	itemName := common.GetStringParam(req, "item_name", "")
	price := common.GetFloatParam(req, "price", 0)

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	if itemName != "" && price > 0 {
		db.ExecContext(ctx, "INSERT INTO costco_price_history (item_name, price) VALUES (?, ?)", itemName, price)
		return tools.TextResult(fmt.Sprintf("Tracked %s at $%.2f.", itemName, price)), nil
	}

	// Show recent prices
	rows, err := db.QueryContext(ctx, `
		SELECT item_name, price, recorded_at
		FROM costco_price_history ORDER BY recorded_at DESC LIMIT 20
	`)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("Costco Price History")
	headers := []string{"Item", "Price", "Date"}
	var tableRows [][]string
	for rows.Next() {
		var name, date string
		var p float64
		if rows.Scan(&name, &p, &date) == nil {
			tableRows = append(tableRows, []string{name, fmt.Sprintf("$%.2f", p), date})
		}
	}
	if len(tableRows) == 0 {
		md.EmptyList("price records")
	} else {
		md.Table(headers, tableRows)
	}
	return tools.TextResult(md.String()), nil
}

// --- Bills Handlers ---

func handleBillsList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	rows, err := db.QueryContext(ctx, `
		SELECT hb.id, hb.name, hb.amount, hb.due_day, hb.frequency, hb.split_type, hb.category, hb.auto_pay,
		       COALESCE(hm.name, '') AS responsible
		FROM house_bills hb
		LEFT JOIN household_members hm ON hm.id = hb.responsible_member
		ORDER BY hb.due_day ASC
	`)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("House Bills")
	headers := []string{"ID", "Bill", "Amount", "Due Day", "Freq", "Split", "Category", "Auto", "Owner"}
	var tableRows [][]string
	var totalMonthly float64

	for rows.Next() {
		var id, dueDay int
		var name, freq, splitType, category, responsible string
		var amount float64
		var autoPay int
		if rows.Scan(&id, &name, &amount, &dueDay, &freq, &splitType, &category, &autoPay, &responsible) != nil {
			continue
		}
		monthly := amount
		if freq == "quarterly" {
			monthly = amount / 3
		} else if freq == "annual" {
			monthly = amount / 12
		}
		totalMonthly += monthly
		autoStr := ""
		if autoPay == 1 {
			autoStr = "Y"
		}
		tableRows = append(tableRows, []string{
			fmt.Sprintf("%d", id), name, fmt.Sprintf("$%.2f", amount),
			fmt.Sprintf("%d", dueDay), freq, splitType, category, autoStr, responsible,
		})
	}

	if len(tableRows) == 0 {
		md.EmptyList("house bills")
	} else {
		md.Table(headers, tableRows)
		md.KeyValue("Total monthly", fmt.Sprintf("$%.2f", totalMonthly))
		var memberCount int
		db.QueryRowContext(ctx, "SELECT COUNT(*) FROM household_members WHERE is_active = 1").Scan(&memberCount)
		if memberCount > 0 {
			md.KeyValue("Per person/month", fmt.Sprintf("$%.2f", totalMonthly/float64(memberCount)))
		}
	}
	return tools.TextResult(md.String()), nil
}

func handleBillsAdd(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := common.GetStringParam(req, "name", "")
	if name == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "name is required"), nil
	}
	amount := common.GetFloatParam(req, "amount", 0)
	if amount <= 0 {
		return common.CodedErrorResultf(common.ErrInvalidParam, "amount must be positive"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	dueDay := common.GetIntParam(req, "due_day", 1)
	freq := common.GetStringParam(req, "frequency", "monthly")
	splitType := common.GetStringParam(req, "split_type", "equal")
	category := common.GetStringParam(req, "category", "utilities")
	responsible := common.GetStringParam(req, "member_id", "")

	result, err := db.ExecContext(ctx,
		"INSERT INTO house_bills (name, amount, due_day, frequency, split_type, category, responsible_member) VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, ''))",
		name, amount, dueDay, freq, splitType, category, responsible)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	id, _ := result.LastInsertId()
	return tools.TextResult(fmt.Sprintf("Bill **%s** added (ID: %d) — $%.2f %s, due day %d.", name, id, amount, freq, dueDay)), nil
}

func handleBillsSplit(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	period := common.GetStringParam(req, "period", time.Now().Format("2006-01"))

	// Get all bills and calculate per-member splits
	billRows, err := db.QueryContext(ctx, "SELECT id, name, amount, split_type, responsible_member FROM house_bills")
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer billRows.Close()

	// Get active members
	var members []struct{ id, name string }
	mRows, _ := db.QueryContext(ctx, "SELECT id, name FROM household_members WHERE is_active = 1")
	if mRows != nil {
		defer mRows.Close()
		for mRows.Next() {
			var id, name string
			if mRows.Scan(&id, &name) == nil {
				members = append(members, struct{ id, name string }{id, name})
			}
		}
	}

	type memberDebt struct {
		name  string
		total float64
		bills []string
	}
	debts := make(map[string]*memberDebt)
	for _, m := range members {
		debts[m.id] = &memberDebt{name: m.name}
	}

	for billRows.Next() {
		var id int
		var name, splitType string
		var amount float64
		var responsible sql.NullString
		if billRows.Scan(&id, &name, &amount, &splitType, &responsible) != nil {
			continue
		}

		switch splitType {
		case "equal":
			perPerson := amount / float64(len(members))
			for _, m := range members {
				if d, ok := debts[m.id]; ok {
					d.total += perPerson
					d.bills = append(d.bills, fmt.Sprintf("%s: $%.2f", name, perPerson))
				}
			}
		case "single":
			if responsible.Valid {
				if d, ok := debts[responsible.String]; ok {
					d.total += amount
					d.bills = append(d.bills, fmt.Sprintf("%s: $%.2f", name, amount))
				}
			}
		}
	}

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Bill Split — %s", period))
	headers := []string{"Member", "Total Owed", "Bills"}
	var tableRows [][]string
	for _, m := range members {
		d := debts[m.id]
		billList := strings.Join(d.bills, "; ")
		tableRows = append(tableRows, []string{d.name, fmt.Sprintf("$%.2f", d.total), billList})
	}
	md.Table(headers, tableRows)

	return tools.TextResult(md.String()), nil
}

func handleBillsSettle(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	memberID := getMemberID(req)
	amount := common.GetFloatParam(req, "amount", 0)
	if amount <= 0 {
		return common.CodedErrorResultf(common.ErrInvalidParam, "amount is required"), nil
	}
	venmoRef := common.GetStringParam(req, "venmo_ref", "")
	period := common.GetStringParam(req, "period", time.Now().Format("2006-01"))

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	// Record generic settlement
	_, err = db.ExecContext(ctx,
		"INSERT INTO bill_payments (bill_id, member_id, amount, period, paid, paid_at, venmo_ref) VALUES (0, ?, ?, ?, 1, datetime('now'), ?)",
		memberID, amount, period, venmoRef)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	name := memberName(ctx, db, memberID)
	return tools.TextResult(fmt.Sprintf("Recorded $%.2f payment from %s for %s. Venmo: %s", amount, name, period, venmoRef)), nil
}

func handleBillsHistory(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	rows, err := db.QueryContext(ctx, `
		SELECT bp.id, hm.name, bp.amount, bp.period, bp.paid_at, bp.venmo_ref
		FROM bill_payments bp
		JOIN household_members hm ON hm.id = bp.member_id
		ORDER BY bp.paid_at DESC LIMIT 20
	`)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("Payment History")
	headers := []string{"ID", "Member", "Amount", "Period", "Paid At", "Venmo"}
	var tableRows [][]string
	for rows.Next() {
		var id int
		var name, period, venmo string
		var amount float64
		var paidAt sql.NullString
		if rows.Scan(&id, &name, &amount, &period, &paidAt, &venmo) != nil {
			continue
		}
		paid := ""
		if paidAt.Valid {
			paid = paidAt.String
		}
		tableRows = append(tableRows, []string{
			fmt.Sprintf("%d", id), name, fmt.Sprintf("$%.2f", amount), period, paid, venmo,
		})
	}
	if len(tableRows) == 0 {
		md.EmptyList("payments")
	} else {
		md.Table(headers, tableRows)
	}
	return tools.TextResult(md.String()), nil
}

// --- Chores Handlers ---

func handleChoresList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	weekOf := currentWeekStart()

	rows, err := db.QueryContext(ctx, `
		SELECT ca.id, cd.name, hm.name AS assigned_to, ca.completed, cd.karma_points
		FROM chore_assignments ca
		JOIN chore_definitions cd ON cd.id = ca.chore_id
		JOIN household_members hm ON hm.id = ca.member_id
		WHERE ca.week_of = ?
		ORDER BY ca.completed ASC, cd.name ASC
	`, weekOf)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Chores — Week of %s", weekOf))
	headers := []string{"ID", "Chore", "Assigned To", "Done", "Karma"}
	var tableRows [][]string
	for rows.Next() {
		var id, completed, karma int
		var choreName, assignedTo string
		if rows.Scan(&id, &choreName, &assignedTo, &completed, &karma) != nil {
			continue
		}
		done := "No"
		if completed == 1 {
			done = "Yes"
		}
		tableRows = append(tableRows, []string{
			fmt.Sprintf("%d", id), choreName, assignedTo, done, fmt.Sprintf("%d", karma),
		})
	}
	if len(tableRows) == 0 {
		md.Text("No chore assignments for this week. Use chores/rotate to generate assignments.")
	} else {
		md.Table(headers, tableRows)
	}
	return tools.TextResult(md.String()), nil
}

func handleChoresAdd(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := common.GetStringParam(req, "name", "")
	if name == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "name is required"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	desc := common.GetStringParam(req, "body", "")
	freq := common.GetStringParam(req, "frequency", "weekly")
	karma := common.GetIntParam(req, "karma_points", 5)

	result, err := db.ExecContext(ctx,
		"INSERT INTO chore_definitions (name, description, frequency, karma_points) VALUES (?, ?, ?, ?)",
		name, desc, freq, karma)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	id, _ := result.LastInsertId()
	return tools.TextResult(fmt.Sprintf("Chore **%s** added (ID: %d) — %s, %d karma.", name, id, freq, karma)), nil
}

func handleChoresRotate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	weekOf := currentWeekStart()

	// Get all chores and active members
	var chores []struct{ id int; name string }
	cRows, _ := db.QueryContext(ctx, "SELECT id, name FROM chore_definitions ORDER BY id")
	if cRows != nil {
		defer cRows.Close()
		for cRows.Next() {
			var id int
			var name string
			if cRows.Scan(&id, &name) == nil {
				chores = append(chores, struct{ id int; name string }{id, name})
			}
		}
	}

	var memberIDs []string
	mRows, _ := db.QueryContext(ctx, "SELECT id FROM household_members WHERE is_active = 1 ORDER BY id")
	if mRows != nil {
		defer mRows.Close()
		for mRows.Next() {
			var id string
			if mRows.Scan(&id) == nil {
				memberIDs = append(memberIDs, id)
			}
		}
	}

	if len(chores) == 0 || len(memberIDs) == 0 {
		return tools.TextResult("No chores or members to assign."), nil
	}

	// Round-robin assignment
	assigned := 0
	for i, chore := range chores {
		memberIdx := i % len(memberIDs)
		_, err := db.ExecContext(ctx,
			"INSERT OR IGNORE INTO chore_assignments (chore_id, member_id, week_of) VALUES (?, ?, ?)",
			chore.id, memberIDs[memberIdx], weekOf)
		if err == nil {
			assigned++
		}
	}

	return tools.TextResult(fmt.Sprintf("Assigned %d chores for week of %s.", assigned, weekOf)), nil
}

func handleChoresComplete(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	assignmentID := int64(common.GetIntParam(req, "assignment_id", 0))
	if assignmentID == 0 {
		return common.CodedErrorResultf(common.ErrInvalidParam, "assignment_id is required"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	// Get karma points and member
	var memberID string
	var karma int
	err = db.QueryRowContext(ctx, `
		SELECT ca.member_id, cd.karma_points
		FROM chore_assignments ca JOIN chore_definitions cd ON cd.id = ca.chore_id
		WHERE ca.id = ?`, assignmentID).Scan(&memberID, &karma)
	if err != nil {
		return common.CodedErrorResultf(common.ErrNotFound, "assignment %d not found", assignmentID), nil
	}

	db.ExecContext(ctx,
		"UPDATE chore_assignments SET completed = 1, completed_at = datetime('now') WHERE id = ?", assignmentID)
	db.ExecContext(ctx,
		"UPDATE household_members SET karma_score = karma_score + ? WHERE id = ?", karma, memberID)

	name := memberName(ctx, db, memberID)
	return tools.TextResult(fmt.Sprintf("Chore completed by %s (+%d karma).", name, karma)), nil
}

// --- House Handlers ---

func handleHouseMembers(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	rows, err := db.QueryContext(ctx,
		"SELECT id, name, phone, venmo_handle, karma_score, is_active FROM household_members ORDER BY karma_score DESC")
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("ArtHouse Members")
	headers := []string{"Name", "Karma", "Venmo", "Phone", "Active"}
	var tableRows [][]string
	for rows.Next() {
		var id, name, phone, venmo string
		var karma, active int
		if rows.Scan(&id, &name, &phone, &venmo, &karma, &active) != nil {
			continue
		}
		activeStr := "Yes"
		if active == 0 {
			activeStr = "No"
		}
		tableRows = append(tableRows, []string{name, fmt.Sprintf("%d", karma), venmo, phone, activeStr})
	}
	md.Table(headers, tableRows)
	return tools.TextResult(md.String()), nil
}

func handleHouseAnnouncements(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	title := common.GetStringParam(req, "title", "")
	body := common.GetStringParam(req, "body", "")

	// If posting a new announcement
	if title != "" {
		memberID := getMemberID(req)
		priority := common.GetStringParam(req, "priority", "normal")
		_, err := db.ExecContext(ctx,
			"INSERT INTO house_announcements (author_id, title, body, priority) VALUES (?, ?, ?, ?)",
			memberID, title, body, priority)
		if err != nil {
			return common.CodedErrorResult(common.ErrDBError, err), nil
		}
		return tools.TextResult(fmt.Sprintf("Announcement posted: **%s**", title)), nil
	}

	// List announcements
	rows, err := db.QueryContext(ctx, `
		SELECT ha.id, hm.name, ha.title, ha.body, ha.priority, ha.created_at
		FROM house_announcements ha
		JOIN household_members hm ON hm.id = ha.author_id
		WHERE ha.expires_at IS NULL OR ha.expires_at > datetime('now')
		ORDER BY ha.created_at DESC LIMIT 10
	`)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("House Announcements")
	var announcements []string
	for rows.Next() {
		var id int
		var author, title, body, priority, created string
		if rows.Scan(&id, &author, &title, &body, &priority, &created) != nil {
			continue
		}
		prefix := ""
		if priority == "urgent" {
			prefix = "[URGENT] "
		}
		entry := fmt.Sprintf("**%s%s** — %s (%s)", prefix, title, author, created)
		if body != "" {
			entry += "\n  " + body
		}
		announcements = append(announcements, entry)
	}
	if len(announcements) == 0 {
		md.EmptyList("announcements")
	} else {
		md.List(announcements)
	}
	return tools.TextResult(md.String()), nil
}

func handleHouseMaintenance(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	title := common.GetStringParam(req, "title", "")
	maintID := int64(common.GetIntParam(req, "maintenance_id", 0))
	status := common.GetStringParam(req, "status", "")

	// Update existing request
	if maintID > 0 && status != "" {
		db.ExecContext(ctx, "UPDATE maintenance_requests SET status = ? WHERE id = ?", status, maintID)
		if status == "completed" {
			db.ExecContext(ctx, "UPDATE maintenance_requests SET completed_at = datetime('now') WHERE id = ?", maintID)
		}
		return tools.TextResult(fmt.Sprintf("Maintenance #%d updated to %s.", maintID, status)), nil
	}

	// Create new request
	if title != "" {
		memberID := getMemberID(req)
		desc := common.GetStringParam(req, "body", "")
		priority := common.GetStringParam(req, "priority", "normal")
		result, err := db.ExecContext(ctx,
			"INSERT INTO maintenance_requests (title, description, reported_by, priority) VALUES (?, ?, ?, ?)",
			title, desc, memberID, priority)
		if err != nil {
			return common.CodedErrorResult(common.ErrDBError, err), nil
		}
		id, _ := result.LastInsertId()
		return tools.TextResult(fmt.Sprintf("Maintenance request #%d created: **%s**", id, title)), nil
	}

	// List requests
	rows, err := db.QueryContext(ctx, `
		SELECT mr.id, mr.title, mr.priority, mr.status, hm.name AS reporter, mr.created_at
		FROM maintenance_requests mr
		JOIN household_members hm ON hm.id = mr.reported_by
		WHERE mr.status IN ('open', 'in_progress')
		ORDER BY CASE mr.priority WHEN 'urgent' THEN 0 WHEN 'normal' THEN 1 ELSE 2 END, mr.created_at DESC
	`)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("Maintenance Requests")
	headers := []string{"ID", "Title", "Priority", "Status", "Reporter", "Created"}
	var tableRows [][]string
	for rows.Next() {
		var id int
		var title, priority, status, reporter, created string
		if rows.Scan(&id, &title, &priority, &status, &reporter, &created) != nil {
			continue
		}
		tableRows = append(tableRows, []string{
			fmt.Sprintf("%d", id), title, priority, status, reporter, created,
		})
	}
	if len(tableRows) == 0 {
		md.Text("No open maintenance requests.")
	} else {
		md.Table(headers, tableRows)
	}
	return tools.TextResult(md.String()), nil
}

func handleHouseDashboard(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	md := common.NewMarkdownBuilder().Title("ArtHouse Dashboard")

	// Karma leaderboard
	md.Section("Karma Leaderboard")
	karmaRows, _ := db.QueryContext(ctx,
		"SELECT name, karma_score FROM household_members WHERE is_active = 1 ORDER BY karma_score DESC")
	if karmaRows != nil {
		defer karmaRows.Close()
		var leaders []string
		rank := 1
		for karmaRows.Next() {
			var name string
			var karma int
			if karmaRows.Scan(&name, &karma) == nil {
				medal := ""
				if rank == 1 {
					medal = " [1st]"
				}
				leaders = append(leaders, fmt.Sprintf("%s: %d%s", name, karma, medal))
				rank++
			}
		}
		md.List(leaders)
	}

	// Pending groceries
	var pendingGroceries int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM grocery_items WHERE status = 'pending'").Scan(&pendingGroceries)
	md.Section("Quick Stats")
	md.KeyValue("Pending grocery items", fmt.Sprintf("%d", pendingGroceries))

	// Open maintenance
	var openMaint int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM maintenance_requests WHERE status IN ('open', 'in_progress')").Scan(&openMaint)
	md.KeyValue("Open maintenance", fmt.Sprintf("%d", openMaint))

	// This week's chores
	weekOf := currentWeekStart()
	var totalChores, doneChores int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM chore_assignments WHERE week_of = ?", weekOf).Scan(&totalChores)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM chore_assignments WHERE week_of = ? AND completed = 1", weekOf).Scan(&doneChores)
	md.KeyValue("Chores this week", fmt.Sprintf("%d / %d done", doneChores, totalChores))

	// Recent announcements
	var recentAnnouncements int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM house_announcements WHERE created_at >= datetime('now', '-7 days')").Scan(&recentAnnouncements)
	md.KeyValue("Recent announcements", fmt.Sprintf("%d (last 7 days)", recentAnnouncements))

	return tools.TextResult(md.String()), nil
}

// --- Budget Handlers ---

func handleBudgetOverview(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	period := time.Now().Format("2006-01")
	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("House Budget — %s", period))

	// Monthly bills total
	var monthlyBills float64
	db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(CASE
			WHEN frequency = 'monthly' THEN amount
			WHEN frequency = 'quarterly' THEN amount/3
			WHEN frequency = 'annual' THEN amount/12
		END), 0) FROM house_bills
	`).Scan(&monthlyBills)
	md.KeyValue("Monthly bills", fmt.Sprintf("$%.2f", monthlyBills))

	var memberCount int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM household_members WHERE is_active = 1").Scan(&memberCount)
	if memberCount > 0 {
		md.KeyValue("Per person", fmt.Sprintf("$%.2f", monthlyBills/float64(memberCount)))
	}

	// Grocery spending this month
	var grocerySpend float64
	db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(total_cost), 0) FROM grocery_trips
		WHERE status = 'completed' AND completed_at >= ?
	`, period+"-01").Scan(&grocerySpend)
	md.KeyValue("Grocery spend (month)", fmt.Sprintf("$%.2f", grocerySpend))

	// Savings goals
	goalRows, _ := db.QueryContext(ctx,
		"SELECT name, current_amount, target_amount FROM savings_goals ORDER BY name")
	if goalRows != nil {
		defer goalRows.Close()
		md.Section("Savings Goals")
		var goals []string
		for goalRows.Next() {
			var name string
			var current, target float64
			if goalRows.Scan(&name, &current, &target) == nil {
				pct := 0.0
				if target > 0 {
					pct = (current / target) * 100
				}
				goals = append(goals, fmt.Sprintf("%s: $%.2f / $%.2f (%.0f%%)", name, current, target, pct))
			}
		}
		if len(goals) > 0 {
			md.List(goals)
		}
	}

	return tools.TextResult(md.String()), nil
}

func handleBudgetSavingsGoals(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	name := common.GetStringParam(req, "name", "")
	goalID := int64(common.GetIntParam(req, "goal_id", 0))
	deposit := common.GetFloatParam(req, "deposit", 0)

	// Deposit into existing goal
	if goalID > 0 && deposit > 0 {
		db.ExecContext(ctx,
			"UPDATE savings_goals SET current_amount = current_amount + ?, updated_at = datetime('now') WHERE id = ?",
			deposit, goalID)
		return tools.TextResult(fmt.Sprintf("Added $%.2f to savings goal #%d.", deposit, goalID)), nil
	}

	// Create new goal
	if name != "" {
		target := common.GetFloatParam(req, "target_amount", 0)
		if target <= 0 {
			return common.CodedErrorResultf(common.ErrInvalidParam, "target_amount is required for new goal"), nil
		}
		category := common.GetStringParam(req, "category", "house")
		result, err := db.ExecContext(ctx,
			"INSERT INTO savings_goals (name, target_amount, category) VALUES (?, ?, ?)",
			name, target, category)
		if err != nil {
			return common.CodedErrorResult(common.ErrDBError, err), nil
		}
		id, _ := result.LastInsertId()
		return tools.TextResult(fmt.Sprintf("Savings goal **%s** created (ID: %d) — target: $%.2f.", name, id, target)), nil
	}

	// List goals
	rows, err := db.QueryContext(ctx,
		"SELECT id, name, target_amount, current_amount, category, due_date FROM savings_goals ORDER BY name")
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("Savings Goals")
	headers := []string{"ID", "Goal", "Current", "Target", "%", "Category", "Due"}
	var tableRows [][]string
	for rows.Next() {
		var id int
		var name, category string
		var target, current float64
		var dueDate sql.NullString
		if rows.Scan(&id, &name, &target, &current, &category, &dueDate) != nil {
			continue
		}
		pct := 0.0
		if target > 0 {
			pct = (current / target) * 100
		}
		due := ""
		if dueDate.Valid {
			due = dueDate.String
		}
		tableRows = append(tableRows, []string{
			fmt.Sprintf("%d", id), name, fmt.Sprintf("$%.2f", current),
			fmt.Sprintf("$%.2f", target), fmt.Sprintf("%.0f%%", pct), category, due,
		})
	}
	if len(tableRows) == 0 {
		md.EmptyList("savings goals")
	} else {
		md.Table(headers, tableRows)
	}
	return tools.TextResult(md.String()), nil
}

// --- Helpers ---

func currentWeekStart() string {
	now := time.Now()
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	monday := now.AddDate(0, 0, -(weekday - 1))
	return monday.Format("2006-01-02")
}
