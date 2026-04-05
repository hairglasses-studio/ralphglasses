// Package growth provides the Growth life category MCP tool.
// Covers spaced repetition, quizzes, coding exercises, and reading queue.
package growth

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/common"
	"github.com/hairglasses-studio/runmylife/internal/srs"
)

type Module struct{}

func (m *Module) Name() string        { return "growth" }
func (m *Module) Description() string { return "Learning & growth: SRS flashcards, quizzes, coding labs, reading" }

var growthHints = map[string]string{
	"srs/review":       "Cards due for review today",
	"srs/add":          "Add a new flashcard",
	"srs/answer":       "Record SRS review result (quality 0-5)",
	"srs/stats":        "SRS review statistics and decay alerts",
	"quiz/daily":       "Get today's quiz questions",
	"quiz/add":         "Add a quiz question",
	"quiz/answer":      "Submit quiz answer",
	"quiz/history":     "Past quiz performance",
	"lab/list":         "List coding exercises",
	"lab/add":          "Add a coding exercise",
	"lab/complete":     "Mark exercise complete",
	"lab/streak":       "Current exercise streak",
	"reading/queue":    "View reading queue",
	"reading/add":      "Add to reading queue",
	"reading/complete": "Mark reading complete",
}

func (m *Module) Tools() []tools.ToolDefinition {
	dispatcher := common.NewDispatcher("growth").
		Domain("srs", common.ActionRegistry{
			"review": handleSRSReview,
			"add":    handleSRSAdd,
			"answer": handleSRSAnswer,
			"stats":  handleSRSStats,
		}).
		Domain("quiz", common.ActionRegistry{
			"daily":   handleQuizDaily,
			"add":     handleQuizAdd,
			"answer":  handleQuizAnswer,
			"history": handleQuizHistory,
		}).
		Domain("lab", common.ActionRegistry{
			"list":     handleLabList,
			"add":      handleLabAdd,
			"complete": handleLabComplete,
			"streak":   handleLabStreak,
		}).
		Domain("reading", common.ActionRegistry{
			"queue":    handleReadingQueue,
			"add":      handleReadingAdd,
			"complete": handleReadingComplete,
		})

	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_growth",
				mcp.WithDescription("Growth & learning gateway.\n\n"+
					dispatcher.DescribeActionsWithHints(growthHints)),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Domain: srs, quiz, lab, reading")),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action within domain")),
				// SRS params
				mcp.WithNumber("card_id", mcp.Description("SRS card ID")),
				mcp.WithString("front", mcp.Description("Flashcard front (question)")),
				mcp.WithString("back", mcp.Description("Flashcard back (answer)")),
				mcp.WithString("topic", mcp.Description("Topic/subject")),
				mcp.WithString("tags", mcp.Description("Comma-separated tags")),
				mcp.WithNumber("quality", mcp.Description("SRS review quality (0-5)")),
				// Quiz params
				mcp.WithNumber("question_id", mcp.Description("Quiz question ID")),
				mcp.WithString("question", mcp.Description("Quiz question text")),
				mcp.WithString("answer", mcp.Description("Answer text")),
				mcp.WithString("explanation", mcp.Description("Answer explanation")),
				mcp.WithNumber("difficulty", mcp.Description("Difficulty 1-5")),
				mcp.WithBoolean("correct", mcp.Description("Whether answer was correct")),
				// Lab params
				mcp.WithNumber("exercise_id", mcp.Description("Exercise ID")),
				mcp.WithString("title", mcp.Description("Exercise or reading title")),
				mcp.WithString("platform", mcp.Description("Platform (leetcode, exercism, etc.)")),
				mcp.WithString("language", mcp.Description("Programming language")),
				mcp.WithString("url", mcp.Description("URL")),
				mcp.WithNumber("time_spent", mcp.Description("Time spent in minutes")),
				// Reading params
				mcp.WithNumber("reading_id", mcp.Description("Reading queue item ID")),
				mcp.WithString("author", mcp.Description("Author")),
				mcp.WithString("source", mcp.Description("Source (manual, readwise, reddit, etc.)")),
				mcp.WithNumber("priority", mcp.Description("Priority 1-10")),
				mcp.WithString("notes", mcp.Description("Notes")),
			),
			Handler:     tools.ToolHandlerFunc(dispatcher.Handler()),
			Category:    "life",
			Subcategory: "growth",
			Tags:        []string{"learning", "srs", "flashcards", "quiz", "coding", "reading"},
			Complexity:  tools.ComplexityModerate,
			IsWrite:     true,
			Timeout:     30 * time.Second,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

// --- SRS Handlers ---

func handleSRSReview(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	topic := common.GetStringParam(req, "topic", "")

	query := "SELECT id, front, back, topic, easiness_factor, interval_days, repetitions FROM srs_cards WHERE next_review_at <= datetime('now')"
	var args []interface{}
	if topic != "" {
		query += " AND topic = ?"
		args = append(args, topic)
	}
	query += " ORDER BY next_review_at ASC LIMIT 10"

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("SRS Review")
	var dueCards [][]string
	for rows.Next() {
		var id, intervalDays, reps int
		var front, back, cardTopic string
		var ef float64
		if rows.Scan(&id, &front, &back, &cardTopic, &ef, &intervalDays, &reps) != nil {
			continue
		}
		card := srs.Card{EasinessFactor: ef, IntervalDays: intervalDays, Repetitions: reps}
		decayFlag := ""
		if srs.DecayAlert(card) {
			decayFlag = " [DECAY]"
		}
		dueCards = append(dueCards, []string{
			fmt.Sprintf("%d", id), cardTopic + decayFlag, front, fmt.Sprintf("%.2f", ef), fmt.Sprintf("%d", reps),
		})
	}

	if len(dueCards) == 0 {
		// Show total and next due
		var total int
		var nextDue sql.NullString
		db.QueryRowContext(ctx, "SELECT COUNT(*) FROM srs_cards").Scan(&total)
		db.QueryRowContext(ctx, "SELECT MIN(next_review_at) FROM srs_cards WHERE next_review_at > datetime('now')").Scan(&nextDue)
		md.Text(fmt.Sprintf("No cards due! %d total cards.", total))
		if nextDue.Valid {
			md.KeyValue("Next review", nextDue.String)
		}
	} else {
		md.KeyValue("Cards due", fmt.Sprintf("%d", len(dueCards)))
		md.Table([]string{"ID", "Topic", "Question", "EF", "Reps"}, dueCards)
		md.Text("Use `srs/answer` with card_id and quality (0-5) to record your review.")
	}

	return tools.TextResult(md.String()), nil
}

func handleSRSAdd(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	front := common.GetStringParam(req, "front", "")
	back := common.GetStringParam(req, "back", "")
	if front == "" || back == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "front and back are required"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	topic := common.GetStringParam(req, "topic", "")
	tags := common.GetStringParam(req, "tags", "")
	source := common.GetStringParam(req, "source", "manual")

	result, err := db.ExecContext(ctx,
		"INSERT INTO srs_cards (front, back, topic, tags, source) VALUES (?, ?, ?, ?, ?)",
		front, back, topic, tags, source)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	id, _ := result.LastInsertId()
	return tools.TextResult(fmt.Sprintf("Card #%d added to SRS deck. Topic: %s.", id, topic)), nil
}

func handleSRSAnswer(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cardID := int64(common.GetIntParam(req, "card_id", 0))
	if cardID == 0 {
		return common.CodedErrorResultf(common.ErrInvalidParam, "card_id is required"), nil
	}
	quality := common.GetIntParam(req, "quality", -1)
	if quality < 0 || quality > 5 {
		return common.CodedErrorResultf(common.ErrInvalidParam, "quality must be 0-5"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	// Load card state
	var ef float64
	var intervalDays, reps int
	err = db.QueryRowContext(ctx,
		"SELECT easiness_factor, interval_days, repetitions FROM srs_cards WHERE id = ?", cardID).
		Scan(&ef, &intervalDays, &reps)
	if err != nil {
		return common.CodedErrorResultf(common.ErrNotFound, "card %d not found", cardID), nil
	}

	card := srs.Card{EasinessFactor: ef, IntervalDays: intervalDays, Repetitions: reps}
	result := srs.Review(card, quality)

	// Update card
	db.ExecContext(ctx, `
		UPDATE srs_cards SET easiness_factor = ?, interval_days = ?, repetitions = ?,
		next_review_at = ?, last_reviewed_at = datetime('now') WHERE id = ?`,
		result.NewEF, result.NewInterval, result.NewRepetitions,
		result.NextReview.Format("2006-01-02T15:04:05"), cardID)

	// Record review
	db.ExecContext(ctx,
		"INSERT INTO srs_reviews (card_id, quality) VALUES (?, ?)", cardID, quality)

	// If wrong, auto-create a reinforcement reminder
	correctStr := "Correct"
	if !result.WasCorrect {
		correctStr = "Incorrect (reset)"
	}

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Card #%d Reviewed", cardID))
	md.KeyValue("Quality", fmt.Sprintf("%d/5", quality))
	md.KeyValue("Result", correctStr)
	md.KeyValue("New EF", fmt.Sprintf("%.2f", result.NewEF))
	md.KeyValue("Next review", fmt.Sprintf("in %d day(s)", result.NewInterval))

	return tools.TextResult(md.String()), nil
}

func handleSRSStats(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	md := common.NewMarkdownBuilder().Title("SRS Statistics")

	var total, dueNow int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM srs_cards").Scan(&total)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM srs_cards WHERE next_review_at <= datetime('now')").Scan(&dueNow)
	md.KeyValue("Total cards", fmt.Sprintf("%d", total))
	md.KeyValue("Due now", fmt.Sprintf("%d", dueNow))

	// Today's reviews
	today := time.Now().Format("2006-01-02")
	var reviewsToday, correctToday int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM srs_reviews WHERE reviewed_at >= ?", today).Scan(&reviewsToday)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM srs_reviews WHERE reviewed_at >= ? AND quality >= 3", today).Scan(&correctToday)
	md.KeyValue("Reviews today", fmt.Sprintf("%d (%d correct)", reviewsToday, correctToday))

	// By topic
	topicRows, err := db.QueryContext(ctx, `
		SELECT topic, COUNT(*) as cnt,
		       SUM(CASE WHEN next_review_at <= datetime('now') THEN 1 ELSE 0 END) as due
		FROM srs_cards WHERE topic != '' GROUP BY topic ORDER BY due DESC, cnt DESC LIMIT 10
	`)
	if err == nil {
		defer topicRows.Close()
		md.Section("By Topic")
		var topicTable [][]string
		for topicRows.Next() {
			var topic string
			var cnt, due int
			if topicRows.Scan(&topic, &cnt, &due) == nil {
				topicTable = append(topicTable, []string{topic, fmt.Sprintf("%d", cnt), fmt.Sprintf("%d", due)})
			}
		}
		if len(topicTable) > 0 {
			md.Table([]string{"Topic", "Cards", "Due"}, topicTable)
		}
	}

	// Decay alerts
	decayRows, err := db.QueryContext(ctx,
		"SELECT id, front, topic, easiness_factor FROM srs_cards WHERE easiness_factor < 1.5 AND repetitions > 2 LIMIT 5")
	if err == nil {
		defer decayRows.Close()
		var decayCards []string
		for decayRows.Next() {
			var id int
			var front, topic string
			var ef float64
			if decayRows.Scan(&id, &front, &topic, &ef) == nil {
				decayCards = append(decayCards, fmt.Sprintf("#%d %s (%s) — EF: %.2f", id, common.TruncateWords(front, 40), topic, ef))
			}
		}
		if len(decayCards) > 0 {
			md.Section("Decay Alerts")
			md.List(decayCards)
		}
	}

	return tools.TextResult(md.String()), nil
}

// --- Quiz Handlers ---

func handleQuizDaily(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	topic := common.GetStringParam(req, "topic", "")

	query := "SELECT id, topic, question, difficulty FROM quiz_questions"
	var args []interface{}
	if topic != "" {
		query += " WHERE topic = ?"
		args = append(args, topic)
	}
	// Prioritize least-asked questions
	query += " ORDER BY times_asked ASC, RANDOM() LIMIT 5"

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("Daily Quiz")
	var questions [][]string
	for rows.Next() {
		var id, difficulty int
		var qTopic, question string
		if rows.Scan(&id, &qTopic, &question, &difficulty) == nil {
			questions = append(questions, []string{fmt.Sprintf("%d", id), qTopic, question, fmt.Sprintf("%d/5", difficulty)})
		}
	}

	if len(questions) == 0 {
		md.Text("No quiz questions available. Use quiz/add to create some.")
	} else {
		md.Table([]string{"ID", "Topic", "Question", "Difficulty"}, questions)
		md.Text("Use `quiz/answer` with question_id and correct=true/false to record your answer.")
	}

	return tools.TextResult(md.String()), nil
}

func handleQuizAdd(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	question := common.GetStringParam(req, "question", "")
	answer := common.GetStringParam(req, "answer", "")
	if question == "" || answer == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "question and answer are required"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	topic := common.GetStringParam(req, "topic", "general")
	explanation := common.GetStringParam(req, "explanation", "")
	difficulty := common.GetIntParam(req, "difficulty", 3)
	source := common.GetStringParam(req, "source", "manual")
	url := common.GetStringParam(req, "url", "")

	result, err := db.ExecContext(ctx,
		"INSERT INTO quiz_questions (topic, question, answer, explanation, difficulty, source, external_url) VALUES (?, ?, ?, ?, ?, ?, ?)",
		topic, question, answer, explanation, difficulty, source, url)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	id, _ := result.LastInsertId()
	return tools.TextResult(fmt.Sprintf("Quiz question #%d added. Topic: %s.", id, topic)), nil
}

func handleQuizAnswer(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	questionID := int64(common.GetIntParam(req, "question_id", 0))
	if questionID == 0 {
		return common.CodedErrorResultf(common.ErrInvalidParam, "question_id is required"), nil
	}
	correct := common.GetBoolParam(req, "correct", false)

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	// Update question stats
	if correct {
		db.ExecContext(ctx, "UPDATE quiz_questions SET times_asked = times_asked + 1, times_correct = times_correct + 1 WHERE id = ?", questionID)
	} else {
		db.ExecContext(ctx, "UPDATE quiz_questions SET times_asked = times_asked + 1 WHERE id = ?", questionID)
	}

	// Show correct answer
	var answer, explanation, topic string
	db.QueryRowContext(ctx, "SELECT answer, explanation, topic FROM quiz_questions WHERE id = ?", questionID).
		Scan(&answer, &explanation, &topic)

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Question #%d", questionID))
	if correct {
		md.Text("Correct!")
	} else {
		md.Text("Incorrect.")
		// Auto-create SRS card from wrong answer
		var question string
		db.QueryRowContext(ctx, "SELECT question FROM quiz_questions WHERE id = ?", questionID).Scan(&question)
		if question != "" {
			db.ExecContext(ctx,
				"INSERT INTO srs_cards (front, back, topic, source) VALUES (?, ?, ?, 'quiz_miss')",
				question, answer, topic)
			md.Text("Auto-created SRS card from this miss.")
		}
	}
	md.KeyValue("Answer", answer)
	if explanation != "" {
		md.KeyValue("Explanation", explanation)
	}

	return tools.TextResult(md.String()), nil
}

func handleQuizHistory(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	md := common.NewMarkdownBuilder().Title("Quiz History")

	// Overall stats
	var totalQ, totalAsked, totalCorrect int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM quiz_questions").Scan(&totalQ)
	db.QueryRowContext(ctx, "SELECT COALESCE(SUM(times_asked), 0) FROM quiz_questions").Scan(&totalAsked)
	db.QueryRowContext(ctx, "SELECT COALESCE(SUM(times_correct), 0) FROM quiz_questions").Scan(&totalCorrect)
	md.KeyValue("Total questions", fmt.Sprintf("%d", totalQ))
	md.KeyValue("Total attempts", fmt.Sprintf("%d", totalAsked))
	if totalAsked > 0 {
		md.KeyValue("Accuracy", fmt.Sprintf("%.0f%%", float64(totalCorrect)/float64(totalAsked)*100))
	}

	// By topic
	topicRows, err := db.QueryContext(ctx, `
		SELECT topic, COUNT(*), SUM(times_asked), SUM(times_correct)
		FROM quiz_questions GROUP BY topic ORDER BY SUM(times_asked) DESC LIMIT 10
	`)
	if err == nil {
		defer topicRows.Close()
		var topicTable [][]string
		for topicRows.Next() {
			var topic string
			var cnt, asked, correct int
			if topicRows.Scan(&topic, &cnt, &asked, &correct) == nil {
				acc := ""
				if asked > 0 {
					acc = fmt.Sprintf("%.0f%%", float64(correct)/float64(asked)*100)
				}
				topicTable = append(topicTable, []string{topic, fmt.Sprintf("%d", cnt), fmt.Sprintf("%d", asked), acc})
			}
		}
		if len(topicTable) > 0 {
			md.Section("By Topic")
			md.Table([]string{"Topic", "Questions", "Attempts", "Accuracy"}, topicTable)
		}
	}

	return tools.TextResult(md.String()), nil
}

// --- Lab Handlers ---

func handleLabList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	rows, err := db.QueryContext(ctx, `
		SELECT id, title, platform, language, difficulty, status, url
		FROM lab_exercises ORDER BY
			CASE status WHEN 'pending' THEN 0 WHEN 'in_progress' THEN 1 ELSE 2 END,
			created_at DESC LIMIT 20
	`)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("Coding Exercises")
	var tableRows [][]string
	for rows.Next() {
		var id int
		var title, platform, lang, difficulty, status, url string
		if rows.Scan(&id, &title, &platform, &lang, &difficulty, &status, &url) == nil {
			tableRows = append(tableRows, []string{
				fmt.Sprintf("%d", id), title, platform, lang, difficulty, status,
			})
		}
	}
	if len(tableRows) == 0 {
		md.EmptyList("exercises")
	} else {
		md.Table([]string{"ID", "Title", "Platform", "Lang", "Difficulty", "Status"}, tableRows)
	}
	return tools.TextResult(md.String()), nil
}

func handleLabAdd(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	title := common.GetStringParam(req, "title", "")
	if title == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "title is required"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	platform := common.GetStringParam(req, "platform", "")
	lang := common.GetStringParam(req, "language", "")
	difficulty := common.GetStringParam(req, "difficulty", "medium")
	url := common.GetStringParam(req, "url", "")

	result, err := db.ExecContext(ctx,
		"INSERT INTO lab_exercises (title, platform, language, difficulty, url) VALUES (?, ?, ?, ?, ?)",
		title, platform, lang, difficulty, url)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	id, _ := result.LastInsertId()
	return tools.TextResult(fmt.Sprintf("Exercise #%d added: %s (%s/%s).", id, title, platform, lang)), nil
}

func handleLabComplete(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	exerciseID := int64(common.GetIntParam(req, "exercise_id", 0))
	if exerciseID == 0 {
		return common.CodedErrorResultf(common.ErrInvalidParam, "exercise_id is required"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	timeSpent := common.GetFloatParam(req, "time_spent", 0)
	db.ExecContext(ctx,
		"UPDATE lab_exercises SET status = 'completed', completed_at = datetime('now'), time_spent_minutes = ? WHERE id = ?",
		timeSpent, exerciseID)

	return tools.TextResult(fmt.Sprintf("Exercise #%d completed! Time: %.0f min.", exerciseID, timeSpent)), nil
}

func handleLabStreak(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	// Count consecutive days with completed exercises
	streak := 0
	checkDate := time.Now()
	for i := 0; i < 365; i++ {
		dateStr := checkDate.Format("2006-01-02")
		var count int
		db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM lab_exercises WHERE status = 'completed' AND date(completed_at) = ?", dateStr).Scan(&count)
		if count == 0 {
			break
		}
		streak++
		checkDate = checkDate.AddDate(0, 0, -1)
	}

	var totalCompleted int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM lab_exercises WHERE status = 'completed'").Scan(&totalCompleted)

	md := common.NewMarkdownBuilder().Title("Lab Streak")
	md.KeyValue("Current streak", fmt.Sprintf("%d day(s)", streak))
	md.KeyValue("Total completed", fmt.Sprintf("%d", totalCompleted))

	return tools.TextResult(md.String()), nil
}

// --- Reading Handlers ---

func handleReadingQueue(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	rows, err := db.QueryContext(ctx, `
		SELECT id, title, author, source, type, priority, status
		FROM reading_queue
		WHERE status IN ('queued', 'reading')
		ORDER BY priority DESC, created_at ASC LIMIT 20
	`)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("Reading Queue")
	var tableRows [][]string
	for rows.Next() {
		var id, priority int
		var title, author, source, rType, status string
		if rows.Scan(&id, &title, &author, &source, &rType, &priority, &status) == nil {
			tableRows = append(tableRows, []string{
				fmt.Sprintf("%d", id), title, author, source, rType, fmt.Sprintf("%d", priority), status,
			})
		}
	}
	if len(tableRows) == 0 {
		md.EmptyList("reading items")
	} else {
		md.Table([]string{"ID", "Title", "Author", "Source", "Type", "Priority", "Status"}, tableRows)
	}
	return tools.TextResult(md.String()), nil
}

func handleReadingAdd(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	title := common.GetStringParam(req, "title", "")
	if title == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "title is required"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	author := common.GetStringParam(req, "author", "")
	url := common.GetStringParam(req, "url", "")
	source := common.GetStringParam(req, "source", "manual")
	priority := common.GetIntParam(req, "priority", 5)

	result, err := db.ExecContext(ctx,
		"INSERT INTO reading_queue (title, author, url, source, priority) VALUES (?, ?, ?, ?, ?)",
		title, author, url, source, priority)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	id, _ := result.LastInsertId()
	return tools.TextResult(fmt.Sprintf("Added to reading queue: #%d **%s** (priority: %d).", id, title, priority)), nil
}

func handleReadingComplete(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	readingID := int64(common.GetIntParam(req, "reading_id", 0))
	if readingID == 0 {
		return common.CodedErrorResultf(common.ErrInvalidParam, "reading_id is required"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	notes := common.GetStringParam(req, "notes", "")
	db.ExecContext(ctx,
		"UPDATE reading_queue SET status = 'completed', completed_at = datetime('now'), notes = ? WHERE id = ?",
		notes, readingID)

	return tools.TextResult(fmt.Sprintf("Reading #%d marked complete.", readingID)), nil
}
