package db

import "fmt"

// Migration represents a database migration with version and SQL.
type Migration struct {
	Version int
	SQL     string
}

var migrations = []Migration{
	{
		Version: 1,
		SQL: `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS tasks (
    id TEXT PRIMARY KEY,
    todoist_id TEXT UNIQUE,
    title TEXT NOT NULL,
    description TEXT,
    priority INTEGER DEFAULT 1,
    project TEXT,
    due_date TEXT,
    completed INTEGER DEFAULT 0,
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS gmail_messages (
    id TEXT PRIMARY KEY,
    thread_id TEXT,
    from_addr TEXT,
    subject TEXT,
    snippet TEXT,
    body TEXT,
    timestamp TEXT,
    labels TEXT,
    triaged INTEGER DEFAULT 0
);

CREATE TABLE IF NOT EXISTS calendar_events (
    id TEXT PRIMARY KEY,
    gcal_id TEXT UNIQUE,
    summary TEXT,
    description TEXT,
    start_time TEXT,
    end_time TEXT,
    location TEXT,
    attendees TEXT
);

CREATE TABLE IF NOT EXISTS tool_usage (
    tool_name TEXT PRIMARY KEY,
    invocation_count INTEGER DEFAULT 0,
    last_used_at TEXT
);

CREATE TABLE IF NOT EXISTS tool_metrics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tool_name TEXT NOT NULL,
    duration_ms INTEGER,
    is_error BOOLEAN DEFAULT FALSE,
    error_type TEXT,
    error_message TEXT,
    created_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS sync_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source TEXT NOT NULL,
    status TEXT NOT NULL,
    records_synced INTEGER DEFAULT 0,
    error_message TEXT,
    started_at TEXT DEFAULT (datetime('now')),
    completed_at TEXT
);
`,
	},
	{
		Version: 2,
		SQL: `
CREATE TABLE IF NOT EXISTS sms_conversations (
    id TEXT PRIMARY KEY,
    participant TEXT NOT NULL,
    display_name TEXT,
    last_message_at TEXT,
    message_count INTEGER DEFAULT 0,
    created_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS sms_messages (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL REFERENCES sms_conversations(id),
    sender TEXT NOT NULL,
    body TEXT,
    sent_at TEXT,
    direction TEXT CHECK(direction IN ('incoming', 'outgoing')),
    is_rcs INTEGER DEFAULT 0,
    fetched_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_sms_messages_conversation ON sms_messages(conversation_id);
CREATE INDEX IF NOT EXISTS idx_sms_messages_sent ON sms_messages(sent_at);

CREATE TABLE IF NOT EXISTS discord_servers (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    icon_url TEXT,
    member_count INTEGER,
    cached_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS discord_channels (
    id TEXT PRIMARY KEY,
    server_id TEXT NOT NULL REFERENCES discord_servers(id),
    name TEXT NOT NULL,
    type TEXT,
    topic TEXT,
    cached_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS discord_messages (
    id TEXT PRIMARY KEY,
    channel_id TEXT NOT NULL,
    author TEXT,
    content TEXT,
    sent_at TEXT,
    cached_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_discord_messages_channel ON discord_messages(channel_id);

CREATE TABLE IF NOT EXISTS drive_files (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    mime_type TEXT,
    parent_id TEXT,
    size INTEGER,
    modified_at TEXT,
    shared INTEGER DEFAULT 0,
    web_link TEXT,
    cached_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_drive_files_parent ON drive_files(parent_id);

CREATE TABLE IF NOT EXISTS notion_databases (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT,
    url TEXT,
    cached_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS notion_pages (
    id TEXT PRIMARY KEY,
    database_id TEXT,
    title TEXT NOT NULL,
    properties_json TEXT,
    url TEXT,
    created_at TEXT,
    last_edited_at TEXT,
    cached_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_notion_pages_db ON notion_pages(database_id);

CREATE TABLE IF NOT EXISTS contacts (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    email TEXT,
    phone TEXT,
    source TEXT,
    source_id TEXT,
    notes TEXT,
    tags TEXT,
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_contacts_name ON contacts(name);
CREATE INDEX IF NOT EXISTS idx_contacts_email ON contacts(email);

CREATE TABLE IF NOT EXISTS weather_cache (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    location_key TEXT NOT NULL,
    data_json TEXT NOT NULL,
    forecast_type TEXT CHECK(forecast_type IN ('current', 'daily', 'hourly')),
    fetched_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS habits (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    frequency TEXT DEFAULT 'daily',
    created_at TEXT DEFAULT (datetime('now')),
    archived INTEGER DEFAULT 0
);

CREATE TABLE IF NOT EXISTS habit_completions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    habit_id TEXT NOT NULL REFERENCES habits(id),
    completed_at TEXT DEFAULT (datetime('now')),
    notes TEXT
);
CREATE INDEX IF NOT EXISTS idx_habit_completions_habit ON habit_completions(habit_id);
CREATE INDEX IF NOT EXISTS idx_habit_completions_date ON habit_completions(completed_at);

CREATE TABLE IF NOT EXISTS transactions (
    id TEXT PRIMARY KEY,
    amount REAL NOT NULL,
    category TEXT,
    description TEXT,
    date TEXT NOT NULL,
    type TEXT CHECK(type IN ('income', 'expense')) DEFAULT 'expense',
    created_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_transactions_date ON transactions(date);
CREATE INDEX IF NOT EXISTS idx_transactions_category ON transactions(category);

CREATE TABLE IF NOT EXISTS budgets (
    id TEXT PRIMARY KEY,
    category TEXT NOT NULL UNIQUE,
    monthly_limit REAL NOT NULL,
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now'))
);
`,
	},
	{
		Version: 3,
		SQL: `
CREATE TABLE IF NOT EXISTS spotify_tracks (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    artist TEXT,
    album TEXT,
    duration_ms INTEGER,
    played_at TEXT,
    cached_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_spotify_tracks_played ON spotify_tracks(played_at);

CREATE TABLE IF NOT EXISTS spotify_playlists (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    owner TEXT,
    track_count INTEGER DEFAULT 0,
    public INTEGER DEFAULT 0,
    url TEXT,
    cached_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS reddit_saved (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    subreddit TEXT,
    author TEXT,
    url TEXT,
    permalink TEXT,
    type TEXT CHECK(type IN ('link', 'comment')),
    score INTEGER DEFAULT 0,
    saved_at TEXT,
    cached_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_reddit_saved_subreddit ON reddit_saved(subreddit);

CREATE TABLE IF NOT EXISTS reddit_subscriptions (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    subscribers INTEGER DEFAULT 0,
    description TEXT,
    url TEXT,
    cached_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS ha_entities (
    entity_id TEXT PRIMARY KEY,
    domain TEXT NOT NULL,
    friendly_name TEXT,
    state TEXT,
    attributes_json TEXT,
    last_changed TEXT,
    cached_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_ha_entities_domain ON ha_entities(domain);

CREATE TABLE IF NOT EXISTS ha_automations (
    id TEXT PRIMARY KEY,
    alias TEXT,
    state TEXT,
    last_triggered TEXT,
    cached_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS fitness_activities (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    duration_ms INTEGER,
    calories INTEGER,
    distance REAL,
    start_time TEXT,
    end_time TEXT,
    cached_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_fitness_activities_date ON fitness_activities(start_time);

CREATE TABLE IF NOT EXISTS fitness_daily_stats (
    date TEXT PRIMARY KEY,
    steps INTEGER DEFAULT 0,
    calories INTEGER DEFAULT 0,
    active_minutes INTEGER DEFAULT 0,
    distance REAL DEFAULT 0,
    resting_heart_rate INTEGER,
    cached_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS fitness_sleep (
    id TEXT PRIMARY KEY,
    date TEXT NOT NULL,
    duration_ms INTEGER,
    start_time TEXT,
    end_time TEXT,
    efficiency INTEGER,
    stages_json TEXT,
    cached_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS readwise_books (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    author TEXT,
    source TEXT,
    num_highlights INTEGER DEFAULT 0,
    url TEXT,
    cached_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS readwise_highlights (
    id TEXT PRIMARY KEY,
    book_id TEXT REFERENCES readwise_books(id),
    text TEXT NOT NULL,
    note TEXT,
    location INTEGER,
    url TEXT,
    highlighted_at TEXT,
    cached_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_readwise_highlights_book ON readwise_highlights(book_id);

CREATE TABLE IF NOT EXISTS bluesky_posts (
    uri TEXT PRIMARY KEY,
    cid TEXT,
    author TEXT NOT NULL,
    text TEXT,
    like_count INTEGER DEFAULT 0,
    repost_count INTEGER DEFAULT 0,
    reply_count INTEGER DEFAULT 0,
    created_at TEXT,
    cached_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_bluesky_posts_author ON bluesky_posts(author);

CREATE TABLE IF NOT EXISTS bluesky_follows (
    did TEXT PRIMARY KEY,
    handle TEXT NOT NULL,
    display_name TEXT,
    cached_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS google_task_lists (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    updated_at TEXT,
    cached_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS google_tasks (
    id TEXT PRIMARY KEY,
    task_list_id TEXT REFERENCES google_task_lists(id),
    title TEXT NOT NULL,
    notes TEXT,
    status TEXT,
    due TEXT,
    completed TEXT,
    parent TEXT,
    position TEXT,
    updated_at TEXT,
    cached_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_google_tasks_list ON google_tasks(task_list_id);

CREATE TABLE IF NOT EXISTS clockify_projects (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    client_name TEXT,
    color TEXT,
    archived INTEGER DEFAULT 0,
    cached_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS clockify_entries (
    id TEXT PRIMARY KEY,
    project_id TEXT REFERENCES clockify_projects(id),
    description TEXT,
    start_time TEXT NOT NULL,
    end_time TEXT,
    duration_seconds INTEGER,
    tags_json TEXT,
    billable INTEGER DEFAULT 0,
    cached_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_clockify_entries_project ON clockify_entries(project_id);
CREATE INDEX IF NOT EXISTS idx_clockify_entries_start ON clockify_entries(start_time);
`,
	},
	{
		Version: 4,
		SQL: `
CREATE TABLE IF NOT EXISTS reply_tracker (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    channel TEXT NOT NULL CHECK(channel IN ('sms', 'discord', 'gmail', 'bluesky')),
    channel_message_id TEXT NOT NULL,
    contact_id TEXT NOT NULL,
    contact_name TEXT NOT NULL DEFAULT '',
    message_preview TEXT DEFAULT '',
    received_at TEXT NOT NULL,
    urgency_score REAL DEFAULT 0,
    urgency_reason TEXT DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending', 'snoozed', 'resolved', 'dismissed')),
    snoozed_until TEXT,
    resolved_at TEXT,
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now')),
    UNIQUE(channel, channel_message_id)
);
CREATE INDEX IF NOT EXISTS idx_reply_tracker_status ON reply_tracker(status);
CREATE INDEX IF NOT EXISTS idx_reply_tracker_contact ON reply_tracker(contact_id);
CREATE INDEX IF NOT EXISTS idx_reply_tracker_urgency ON reply_tracker(urgency_score DESC);

CREATE TABLE IF NOT EXISTS contact_importance (
    contact_id TEXT PRIMARY KEY,
    tier TEXT NOT NULL DEFAULT 'normal' CHECK(tier IN ('vip', 'close', 'normal', 'low')),
    default_reply_window_hours REAL DEFAULT 24,
    relationship_type TEXT DEFAULT '',
    last_interaction_at TEXT,
    interaction_count INTEGER DEFAULT 0,
    avg_reply_time_minutes REAL DEFAULT 0,
    ghost_count INTEGER DEFAULT 0,
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS daily_focus (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    date TEXT NOT NULL,
    time_block TEXT DEFAULT '',
    suggestion_type TEXT NOT NULL,
    suggestion_id TEXT DEFAULT '',
    suggestion_text TEXT NOT NULL,
    priority REAL DEFAULT 0.5,
    accepted INTEGER DEFAULT 0,
    created_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_daily_focus_date ON daily_focus(date);
`,
	},
	{
		Version: 5,
		SQL: `
CREATE TABLE IF NOT EXISTS household_members (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    phone TEXT DEFAULT '',
    venmo_handle TEXT DEFAULT '',
    email TEXT DEFAULT '',
    is_active INTEGER DEFAULT 1,
    karma_score INTEGER DEFAULT 50,
    created_at TEXT DEFAULT (datetime('now'))
);

INSERT OR IGNORE INTO household_members (id, name) VALUES
    ('member-jon', 'Jon'),
    ('member-daniel', 'Daniel'),
    ('member-kevin', 'Kevin'),
    ('member-david', 'David'),
    ('member-mitch', 'Mitch');

CREATE TABLE IF NOT EXISTS grocery_items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    quantity TEXT DEFAULT '1',
    category TEXT DEFAULT 'general',
    requested_by TEXT NOT NULL REFERENCES household_members(id),
    is_costco INTEGER DEFAULT 0,
    estimated_price REAL DEFAULT 0,
    trip_id INTEGER,
    status TEXT DEFAULT 'pending' CHECK(status IN ('pending', 'claimed', 'purchased', 'cancelled')),
    claimed_by TEXT REFERENCES household_members(id),
    created_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_grocery_items_status ON grocery_items(status);
CREATE INDEX IF NOT EXISTS idx_grocery_items_trip ON grocery_items(trip_id);

CREATE TABLE IF NOT EXISTS grocery_trips (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    store TEXT DEFAULT 'Costco',
    shopper_id TEXT REFERENCES household_members(id),
    status TEXT DEFAULT 'planned' CHECK(status IN ('planned', 'shopping', 'completed')),
    total_cost REAL DEFAULT 0,
    receipt_notes TEXT DEFAULT '',
    planned_date TEXT,
    completed_at TEXT,
    created_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS grocery_splits (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    trip_id INTEGER NOT NULL REFERENCES grocery_trips(id),
    member_id TEXT NOT NULL REFERENCES household_members(id),
    amount REAL NOT NULL DEFAULT 0,
    paid INTEGER DEFAULT 0,
    paid_at TEXT,
    venmo_ref TEXT DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_grocery_splits_trip ON grocery_splits(trip_id);

CREATE TABLE IF NOT EXISTS house_bills (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    amount REAL NOT NULL,
    due_day INTEGER DEFAULT 1,
    frequency TEXT DEFAULT 'monthly' CHECK(frequency IN ('monthly', 'quarterly', 'annual')),
    split_type TEXT DEFAULT 'equal' CHECK(split_type IN ('equal', 'custom', 'single')),
    responsible_member TEXT REFERENCES household_members(id),
    category TEXT DEFAULT 'utilities',
    auto_pay INTEGER DEFAULT 0,
    created_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS bill_payments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    bill_id INTEGER NOT NULL REFERENCES house_bills(id),
    member_id TEXT NOT NULL REFERENCES household_members(id),
    amount REAL NOT NULL,
    period TEXT NOT NULL,
    paid INTEGER DEFAULT 0,
    paid_at TEXT,
    venmo_ref TEXT DEFAULT '',
    created_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_bill_payments_bill ON bill_payments(bill_id);
CREATE INDEX IF NOT EXISTS idx_bill_payments_period ON bill_payments(period);

CREATE TABLE IF NOT EXISTS chore_definitions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    description TEXT DEFAULT '',
    frequency TEXT DEFAULT 'weekly' CHECK(frequency IN ('daily', 'weekly', 'biweekly', 'monthly')),
    karma_points INTEGER DEFAULT 5,
    created_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS chore_assignments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    chore_id INTEGER NOT NULL REFERENCES chore_definitions(id),
    member_id TEXT NOT NULL REFERENCES household_members(id),
    week_of TEXT NOT NULL,
    completed INTEGER DEFAULT 0,
    completed_at TEXT,
    created_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_chore_assignments_week ON chore_assignments(week_of);
CREATE INDEX IF NOT EXISTS idx_chore_assignments_member ON chore_assignments(member_id);

CREATE TABLE IF NOT EXISTS house_announcements (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    author_id TEXT NOT NULL REFERENCES household_members(id),
    title TEXT NOT NULL,
    body TEXT DEFAULT '',
    priority TEXT DEFAULT 'normal' CHECK(priority IN ('low', 'normal', 'urgent')),
    expires_at TEXT,
    created_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS maintenance_requests (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    description TEXT DEFAULT '',
    reported_by TEXT NOT NULL REFERENCES household_members(id),
    assigned_to TEXT REFERENCES household_members(id),
    status TEXT DEFAULT 'open' CHECK(status IN ('open', 'in_progress', 'completed', 'wont_fix')),
    priority TEXT DEFAULT 'normal' CHECK(priority IN ('low', 'normal', 'urgent')),
    completed_at TEXT,
    created_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS savings_goals (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    target_amount REAL NOT NULL,
    current_amount REAL DEFAULT 0,
    category TEXT DEFAULT 'house',
    due_date TEXT,
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS costco_price_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    item_name TEXT NOT NULL,
    price REAL NOT NULL,
    unit TEXT DEFAULT '',
    recorded_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_costco_prices_item ON costco_price_history(item_name);
`,
	},
	{
		Version: 6,
		SQL: `
CREATE TABLE IF NOT EXISTS srs_cards (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    front TEXT NOT NULL,
    back TEXT NOT NULL,
    topic TEXT DEFAULT '',
    tags TEXT DEFAULT '',
    source TEXT DEFAULT 'manual',
    easiness_factor REAL DEFAULT 2.5,
    interval_days INTEGER DEFAULT 0,
    repetitions INTEGER DEFAULT 0,
    next_review_at TEXT DEFAULT (datetime('now')),
    last_reviewed_at TEXT,
    created_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_srs_cards_next_review ON srs_cards(next_review_at);
CREATE INDEX IF NOT EXISTS idx_srs_cards_topic ON srs_cards(topic);

CREATE TABLE IF NOT EXISTS srs_reviews (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    card_id INTEGER NOT NULL REFERENCES srs_cards(id),
    quality INTEGER NOT NULL,
    response_time_ms INTEGER DEFAULT 0,
    reviewed_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_srs_reviews_card ON srs_reviews(card_id);

CREATE TABLE IF NOT EXISTS quiz_questions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    topic TEXT NOT NULL,
    question TEXT NOT NULL,
    answer TEXT NOT NULL,
    explanation TEXT DEFAULT '',
    difficulty INTEGER DEFAULT 3,
    source TEXT DEFAULT 'manual',
    external_url TEXT DEFAULT '',
    times_asked INTEGER DEFAULT 0,
    times_correct INTEGER DEFAULT 0,
    created_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_quiz_questions_topic ON quiz_questions(topic);

CREATE TABLE IF NOT EXISTS quiz_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    date TEXT NOT NULL,
    topic TEXT DEFAULT '',
    questions_asked INTEGER DEFAULT 0,
    questions_correct INTEGER DEFAULT 0,
    duration_minutes REAL DEFAULT 0,
    created_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS lab_exercises (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    platform TEXT DEFAULT '',
    language TEXT DEFAULT '',
    difficulty TEXT DEFAULT 'medium',
    url TEXT DEFAULT '',
    status TEXT DEFAULT 'pending' CHECK(status IN ('pending', 'in_progress', 'completed', 'skipped')),
    completed_at TEXT,
    time_spent_minutes REAL DEFAULT 0,
    created_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS reading_queue (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    author TEXT DEFAULT '',
    url TEXT DEFAULT '',
    source TEXT DEFAULT 'manual',
    type TEXT DEFAULT 'article',
    priority INTEGER DEFAULT 5,
    status TEXT DEFAULT 'queued' CHECK(status IN ('queued', 'reading', 'completed', 'dropped')),
    notes TEXT DEFAULT '',
    completed_at TEXT,
    created_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_reading_queue_status ON reading_queue(status);
`,
	},
	{
		Version: 7,
		SQL: `
CREATE TABLE IF NOT EXISTS partner_config (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT '',
    updated_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS date_ideas (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    type TEXT DEFAULT 'outing',
    location TEXT DEFAULT '',
    estimated_cost REAL DEFAULT 0,
    weather_preference TEXT DEFAULT 'any' CHECK(weather_preference IN ('any', 'sunny', 'indoor', 'rainy_ok')),
    indoor_outdoor TEXT DEFAULT 'either' CHECK(indoor_outdoor IN ('indoor', 'outdoor', 'either')),
    rating REAL DEFAULT 0,
    times_done INTEGER DEFAULT 0,
    tags TEXT DEFAULT '',
    created_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS date_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    date TEXT NOT NULL,
    location TEXT DEFAULT '',
    notes TEXT DEFAULT '',
    rating INTEGER DEFAULT 0 CHECK(rating BETWEEN 0 AND 10),
    weather TEXT DEFAULT '',
    cost REAL DEFAULT 0,
    idea_id INTEGER REFERENCES date_ideas(id),
    created_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_date_log_date ON date_log(date);

CREATE TABLE IF NOT EXISTS gift_tracker (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    occasion TEXT DEFAULT '',
    budget REAL DEFAULT 0,
    purchased INTEGER DEFAULT 0,
    purchase_date TEXT,
    url TEXT DEFAULT '',
    notes TEXT DEFAULT '',
    created_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS together_time (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    date TEXT NOT NULL,
    hours REAL NOT NULL DEFAULT 0,
    activity_type TEXT DEFAULT 'quality_time',
    quality_rating INTEGER DEFAULT 0 CHECK(quality_rating BETWEEN 0 AND 10),
    notes TEXT DEFAULT '',
    created_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_together_time_date ON together_time(date);
`,
	},
	{
		Version: 8,
		SQL: `
CREATE TABLE IF NOT EXISTS studio_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    started_at TEXT NOT NULL,
    ended_at TEXT,
    duration_minutes REAL DEFAULT 0,
    activity_type TEXT DEFAULT 'general',
    tools_used TEXT DEFAULT '',
    project TEXT DEFAULT '',
    notes TEXT DEFAULT '',
    auto_detected INTEGER DEFAULT 0,
    created_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_studio_sessions_start ON studio_sessions(started_at);

CREATE TABLE IF NOT EXISTS studio_maintenance (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    description TEXT DEFAULT '',
    priority TEXT DEFAULT 'normal' CHECK(priority IN ('low', 'normal', 'urgent')),
    status TEXT DEFAULT 'open' CHECK(status IN ('open', 'in_progress', 'completed')),
    due_date TEXT,
    completed_at TEXT,
    created_at TEXT DEFAULT (datetime('now'))
);
`,
	},
	{
		Version: 9,
		SQL: `
CREATE TABLE IF NOT EXISTS relationship_health (
    contact_id TEXT PRIMARY KEY,
    contact_name TEXT NOT NULL DEFAULT '',
    health_score REAL DEFAULT 50,
    recency_score REAL DEFAULT 0,
    reciprocity_score REAL DEFAULT 0,
    frequency_score REAL DEFAULT 0,
    responsiveness_score REAL DEFAULT 0,
    quality_score REAL DEFAULT 0,
    last_calculated_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS social_circles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    description TEXT DEFAULT '',
    created_at TEXT DEFAULT (datetime('now'))
);

INSERT OR IGNORE INTO social_circles (name, description) VALUES
    ('family', 'Family members'),
    ('arthouse', 'ArtHouse roommates'),
    ('close_friends', 'Close friends'),
    ('work', 'Professional contacts'),
    ('acquaintances', 'Casual acquaintances');

CREATE TABLE IF NOT EXISTS contact_circles (
    contact_id TEXT NOT NULL,
    circle_id INTEGER NOT NULL REFERENCES social_circles(id),
    added_at TEXT DEFAULT (datetime('now')),
    PRIMARY KEY (contact_id, circle_id)
);

CREATE TABLE IF NOT EXISTS outreach_reminders (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    contact_id TEXT NOT NULL,
    contact_name TEXT NOT NULL DEFAULT '',
    frequency_days INTEGER DEFAULT 30,
    last_outreach_at TEXT,
    next_outreach_at TEXT,
    channel_preference TEXT DEFAULT '',
    notes TEXT DEFAULT '',
    active INTEGER DEFAULT 1,
    created_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_outreach_next ON outreach_reminders(next_outreach_at);

CREATE TABLE IF NOT EXISTS mood_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    date TEXT NOT NULL,
    time_of_day TEXT DEFAULT '',
    mood_score INTEGER NOT NULL CHECK(mood_score BETWEEN 1 AND 10),
    energy_level INTEGER DEFAULT 5 CHECK(energy_level BETWEEN 1 AND 10),
    anxiety_level INTEGER DEFAULT 1 CHECK(anxiety_level BETWEEN 1 AND 10),
    sleep_hours REAL DEFAULT 0,
    exercise_done INTEGER DEFAULT 0,
    notes TEXT DEFAULT '',
    tags TEXT DEFAULT '',
    created_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_mood_log_date ON mood_log(date);
`,
	},
	{
		Version: 10,
		SQL: `
CREATE TABLE IF NOT EXISTS subscriptions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    amount REAL NOT NULL,
    frequency TEXT DEFAULT 'monthly' CHECK(frequency IN ('weekly', 'monthly', 'quarterly', 'annual')),
    category TEXT DEFAULT '',
    next_charge_date TEXT,
    auto_renew INTEGER DEFAULT 1,
    cancel_url TEXT DEFAULT '',
    importance TEXT DEFAULT 'keep' CHECK(importance IN ('essential', 'keep', 'review', 'cancel')),
    notes TEXT DEFAULT '',
    source TEXT DEFAULT 'manual',
    active INTEGER DEFAULT 1,
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS paycheck_allocations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    allocation_type TEXT NOT NULL CHECK(allocation_type IN ('fixed', 'percent')),
    amount REAL NOT NULL,
    category TEXT DEFAULT '',
    priority INTEGER DEFAULT 5,
    active INTEGER DEFAULT 1,
    created_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS csv_imports (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source TEXT NOT NULL DEFAULT 'rocket_money',
    filename TEXT DEFAULT '',
    rows_imported INTEGER DEFAULT 0,
    rows_skipped INTEGER DEFAULT 0,
    imported_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_subscriptions_active ON subscriptions(active);
CREATE INDEX IF NOT EXISTS idx_subscriptions_next ON subscriptions(next_charge_date);
`,
	},
	{
		Version: 11,
		SQL: `
CREATE TABLE IF NOT EXISTS notification_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    message TEXT,
    urgency TEXT DEFAULT 'normal',
    source TEXT DEFAULT '',
    channels TEXT DEFAULT '',
    sent_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_notification_log_sent ON notification_log(sent_at);
CREATE INDEX IF NOT EXISTS idx_notification_log_source ON notification_log(source);
`,
	},
	{
		Version: 12,
		SQL: `
-- Task metadata for ADHD support (extends Todoist tasks with executive function data)
CREATE TABLE IF NOT EXISTS task_metadata (
    task_id TEXT PRIMARY KEY,
    activation_energy INTEGER DEFAULT 3 CHECK(activation_energy BETWEEN 1 AND 5),
    first_step_text TEXT DEFAULT '',
    estimated_minutes INTEGER DEFAULT 0,
    energy_required TEXT DEFAULT 'medium' CHECK(energy_required IN ('low', 'medium', 'high')),
    category TEXT DEFAULT '',
    context_tags TEXT DEFAULT '',
    decomposed INTEGER DEFAULT 0,
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now'))
);

-- Focus sessions: track what the user is working on and for how long
CREATE TABLE IF NOT EXISTS focus_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id TEXT,
    category TEXT NOT NULL DEFAULT '',
    started_at TEXT NOT NULL DEFAULT (datetime('now')),
    ended_at TEXT,
    planned_minutes INTEGER DEFAULT 25,
    actual_minutes INTEGER DEFAULT 0,
    interrupted INTEGER DEFAULT 0,
    notes TEXT DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_focus_sessions_started ON focus_sessions(started_at);
CREATE INDEX IF NOT EXISTS idx_focus_sessions_category ON focus_sessions(category);

-- Time checkpoints: proactive time awareness nudges
CREATE TABLE IF NOT EXISTS time_checkpoints (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_type TEXT NOT NULL,
    reference_id TEXT DEFAULT '',
    checkpoint_time TEXT NOT NULL,
    alert_minutes_before INTEGER DEFAULT 15,
    acknowledged INTEGER DEFAULT 0,
    created_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_time_checkpoints_time ON time_checkpoints(checkpoint_time);

-- Daily energy curve: inferred from mood, fitness, and activity patterns
CREATE TABLE IF NOT EXISTS daily_energy_curve (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    date TEXT NOT NULL,
    hour INTEGER NOT NULL CHECK(hour BETWEEN 0 AND 23),
    energy_level INTEGER DEFAULT 5 CHECK(energy_level BETWEEN 1 AND 10),
    source TEXT DEFAULT 'inferred',
    created_at TEXT DEFAULT (datetime('now')),
    UNIQUE(date, hour)
);
CREATE INDEX IF NOT EXISTS idx_energy_curve_date ON daily_energy_curve(date);

-- Daily overwhelm metric: composite score for overwhelm detection
CREATE TABLE IF NOT EXISTS daily_overwhelm_metric (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    date TEXT NOT NULL UNIQUE,
    open_tasks INTEGER DEFAULT 0,
    completion_velocity REAL DEFAULT 0,
    reply_backlog INTEGER DEFAULT 0,
    overdue_count INTEGER DEFAULT 0,
    composite_score REAL DEFAULT 0,
    triage_activated INTEGER DEFAULT 0,
    created_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_overwhelm_date ON daily_overwhelm_metric(date);

-- Context switches: save/restore mental state when switching between tasks
CREATE TABLE IF NOT EXISTS context_switches (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    from_task TEXT DEFAULT '',
    from_category TEXT DEFAULT '',
    to_task TEXT DEFAULT '',
    to_category TEXT DEFAULT '',
    context_snapshot TEXT DEFAULT '',
    switch_cost_minutes INTEGER DEFAULT 0,
    switched_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_context_switches_time ON context_switches(switched_at);

-- Achievement milestones: dopamine scaffolding
CREATE TABLE IF NOT EXISTS achievement_milestones (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    achievement_type TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT DEFAULT '',
    category TEXT DEFAULT '',
    value INTEGER DEFAULT 0,
    achieved_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_achievements_type ON achievement_milestones(achievement_type);
CREATE INDEX IF NOT EXISTS idx_achievements_date ON achievement_milestones(achieved_at);
`,
	},
	{
		Version: 13,
		SQL: `
-- Slack channels
CREATE TABLE IF NOT EXISTS slack_channels (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    topic TEXT NOT NULL DEFAULT '',
    purpose TEXT NOT NULL DEFAULT '',
    member_count INTEGER DEFAULT 0,
    is_archived INTEGER DEFAULT 0,
    fetched_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Slack channel messages
CREATE TABLE IF NOT EXISTS slack_channel_messages (
    id TEXT PRIMARY KEY,
    channel_id TEXT NOT NULL,
    user_id TEXT NOT NULL DEFAULT '',
    user_name TEXT NOT NULL DEFAULT '',
    text TEXT NOT NULL DEFAULT '',
    timestamp TEXT NOT NULL DEFAULT '',
    thread_ts TEXT NOT NULL DEFAULT '',
    reaction_count INTEGER DEFAULT 0,
    reactions TEXT NOT NULL DEFAULT '[]',
    fetched_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_slack_messages_channel ON slack_channel_messages(channel_id);
CREATE INDEX IF NOT EXISTS idx_slack_messages_ts ON slack_channel_messages(timestamp);

-- Cross-platform conversation links: thread linking across channels
CREATE TABLE IF NOT EXISTS conversation_links (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_channel TEXT NOT NULL,
    source_conversation_id TEXT NOT NULL,
    target_channel TEXT NOT NULL,
    target_conversation_id TEXT NOT NULL,
    link_type TEXT NOT NULL DEFAULT 'related',
    confidence REAL DEFAULT 1.0,
    created_at TEXT DEFAULT (datetime('now')),
    UNIQUE(source_channel, source_conversation_id, target_channel, target_conversation_id)
);
CREATE INDEX IF NOT EXISTS idx_conv_links_source ON conversation_links(source_channel, source_conversation_id);
CREATE INDEX IF NOT EXISTS idx_conv_links_target ON conversation_links(target_channel, target_conversation_id);

-- Reply batch windows: track when reply sessions should occur
CREATE TABLE IF NOT EXISTS reply_batch_windows (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    window_name TEXT NOT NULL,
    start_hour INTEGER NOT NULL CHECK(start_hour BETWEEN 0 AND 23),
    end_hour INTEGER NOT NULL CHECK(end_hour BETWEEN 0 AND 23),
    days_of_week TEXT NOT NULL DEFAULT 'Mon,Tue,Wed,Thu,Fri',
    enabled INTEGER DEFAULT 1,
    created_at TEXT DEFAULT (datetime('now'))
);

-- Seed default reply batch windows (morning/afternoon/evening)
INSERT OR IGNORE INTO reply_batch_windows (id, window_name, start_hour, end_hour) VALUES
    (1, 'morning', 8, 9),
    (2, 'afternoon', 14, 15),
    (3, 'evening', 19, 20);
`,
	},
	{
		Version: 14,
		SQL: `
-- Weekly review snapshots
CREATE TABLE IF NOT EXISTS weekly_review_snapshots (
    week_of TEXT PRIMARY KEY,
    tasks_completed INTEGER DEFAULT 0,
    tasks_created INTEGER DEFAULT 0,
    habits_completed INTEGER DEFAULT 0,
    habits_total INTEGER DEFAULT 0,
    habit_rate REAL DEFAULT 0,
    mood_avg REAL DEFAULT 0,
    energy_avg REAL DEFAULT 0,
    sleep_avg_hours REAL DEFAULT 0,
    replies_sent INTEGER DEFAULT 0,
    replies_overdue INTEGER DEFAULT 0,
    emails_triaged INTEGER DEFAULT 0,
    calendar_events INTEGER DEFAULT 0,
    spend_total REAL DEFAULT 0,
    focus_minutes INTEGER DEFAULT 0,
    overwhelm_days INTEGER DEFAULT 0,
    good_enough_days INTEGER DEFAULT 0,
    social_outreaches INTEGER DEFAULT 0,
    srs_reviews INTEGER DEFAULT 0,
    journal_entries INTEGER DEFAULT 0,
    category_breakdown TEXT NOT NULL DEFAULT '{}',
    created_at TEXT DEFAULT (datetime('now'))
);

-- Monthly review snapshots
CREATE TABLE IF NOT EXISTS monthly_review_snapshots (
    month TEXT PRIMARY KEY,
    week_count INTEGER DEFAULT 0,
    tasks_completed INTEGER DEFAULT 0,
    habit_streak_max INTEGER DEFAULT 0,
    habit_rate_avg REAL DEFAULT 0,
    mood_trend TEXT NOT NULL DEFAULT 'stable',
    energy_trend TEXT NOT NULL DEFAULT 'stable',
    spend_total REAL DEFAULT 0,
    spend_vs_budget REAL DEFAULT 0,
    top_categories TEXT NOT NULL DEFAULT '[]',
    overwhelm_days INTEGER DEFAULT 0,
    good_enough_days INTEGER DEFAULT 0,
    wins TEXT NOT NULL DEFAULT '[]',
    subscription_spend REAL DEFAULT 0,
    created_at TEXT DEFAULT (datetime('now'))
);
`,
	},
	{
		Version: 15,
		SQL: `
-- Event bus persistence
CREATE TABLE IF NOT EXISTS events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_type TEXT NOT NULL,
    payload TEXT NOT NULL DEFAULT '{}',
    source TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_events_type_created ON events(event_type, created_at);
`,
	},
	{
		Version: 16,
		SQL: `
-- Intelligence engine suggestion persistence
CREATE TABLE IF NOT EXISTS intelligence_suggestions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    category TEXT NOT NULL,
    priority REAL NOT NULL DEFAULT 0.0,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    action_hint TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'engine',
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_suggestions_created ON intelligence_suggestions(created_at);
`,
	},
	{
		Version: 17,
		SQL: `
ALTER TABLE habits ADD COLUMN current_streak INTEGER DEFAULT 0;
`,
	},
}

// Migrate runs all pending migrations.
func (d *DB) Migrate() error {
	// Ensure schema_migrations exists
	_, err := d.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	for _, m := range migrations {
		var count int
		err := d.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", m.Version).Scan(&count)
		if err != nil {
			return fmt.Errorf("check migration %d: %w", m.Version, err)
		}
		if count > 0 {
			continue
		}

		if _, err := d.Exec(m.SQL); err != nil {
			return fmt.Errorf("run migration %d: %w", m.Version, err)
		}
		if _, err := d.Exec("INSERT INTO schema_migrations (version) VALUES (?)", m.Version); err != nil {
			return fmt.Errorf("record migration %d: %w", m.Version, err)
		}
	}
	return nil
}
