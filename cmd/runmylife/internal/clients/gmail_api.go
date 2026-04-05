package clients

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// GmailMessage represents a Gmail message with extracted fields.
type GmailMessage struct {
	ID        string
	ThreadID  string
	Subject   string
	From      string
	To        string
	Date      time.Time
	Snippet   string
	Body      string
	Labels    string
	IsRead    bool
	FetchedAt time.Time
}

// GmailDraft represents a Gmail draft with metadata.
type GmailDraft struct {
	ID        string
	MessageID string
	ThreadID  string
	Subject   string
	To        string
}

// GmailAttachment represents an attachment on a Gmail message.
type GmailAttachment struct {
	MessageID string
	PartID    string
	Filename  string
	MimeType  string
	Size      int64
}

// HistoryResult contains the result of a FetchHistory call.
type HistoryResult struct {
	NewMessageIDs   []string
	LatestHistoryId uint64
}

// ErrHistoryExpired indicates the provided historyId is too old and a full sync is needed.
var ErrHistoryExpired = fmt.Errorf("gmail history id expired, full sync required")

// GmailAPIClient wraps Google's Gmail API for live email access.
type GmailAPIClient struct {
	service *gmail.Service
	Account string
}

// NewGmailAPIClient creates a client for the default (personal) account.
func NewGmailAPIClient(ctx context.Context) (*GmailAPIClient, error) {
	return NewGmailAPIClientForAccount(ctx, "")
}

// NewGmailAPIClientForAccount creates a client for a named account.
func NewGmailAPIClientForAccount(ctx context.Context, account string) (*GmailAPIClient, error) {
	credPath := GoogleCredentialsPathForAccount(account)

	if account != "" && account != "personal" {
		if _, err := os.Stat(credPath); os.IsNotExist(err) {
			credPath = GoogleCredentialsPathForAccount("")
		}
	}

	oauthConfig, err := LoadGoogleCredentials(credPath, DefaultGmailScopes()...)
	if err != nil {
		return nil, fmt.Errorf("load credentials (place OAuth JSON at %s): %w", credPath, err)
	}

	token, err := LoadGoogleTokenForAccount(account)
	if err != nil {
		return nil, err
	}

	ts := oauthConfig.TokenSource(ctx, token)
	savingTS := &savingTokenSource{base: ts, lastToken: token, account: account}

	svc, err := gmail.NewService(ctx, option.WithTokenSource(savingTS))
	if err != nil {
		return nil, fmt.Errorf("create gmail service: %w", err)
	}

	acctName := account
	if acctName == "" {
		acctName = "personal"
	}
	return &GmailAPIClient{service: svc, Account: acctName}, nil
}

// FetchMessages queries Gmail for messages matching the query and returns full message data.
func (c *GmailAPIClient) FetchMessages(ctx context.Context, query string, maxResults int64) ([]*GmailMessage, error) {
	if maxResults <= 0 {
		maxResults = 20
	}
	if maxResults > 500 {
		maxResults = 500
	}

	var allIDs []string
	var pageToken string
	remaining := maxResults

	for remaining > 0 {
		pageSize := remaining
		if pageSize > 100 {
			pageSize = 100
		}
		listCall := c.service.Users.Messages.List("me").Q(query).MaxResults(pageSize)
		if pageToken != "" {
			listCall = listCall.PageToken(pageToken)
		}
		resp, err := listCall.Context(ctx).Do()
		if err != nil {
			return nil, fmt.Errorf("list messages: %w", err)
		}
		for _, m := range resp.Messages {
			allIDs = append(allIDs, m.Id)
		}
		remaining -= int64(len(resp.Messages))
		pageToken = resp.NextPageToken
		if pageToken == "" || len(resp.Messages) == 0 {
			break
		}
	}

	if len(allIDs) == 0 {
		return nil, nil
	}

	return c.fetchMessagesByIDs(ctx, allIDs, "full")
}

// FetchMessageHeaders queries Gmail for messages and returns header-only data.
func (c *GmailAPIClient) FetchMessageHeaders(ctx context.Context, query string, maxResults int64) ([]*GmailMessage, error) {
	if maxResults <= 0 {
		maxResults = 20
	}
	if maxResults > 500 {
		maxResults = 500
	}

	var allIDs []string
	var pageToken string
	remaining := maxResults

	for remaining > 0 {
		pageSize := remaining
		if pageSize > 100 {
			pageSize = 100
		}
		listCall := c.service.Users.Messages.List("me").Q(query).MaxResults(pageSize)
		if pageToken != "" {
			listCall = listCall.PageToken(pageToken)
		}
		resp, err := listCall.Context(ctx).Do()
		if err != nil {
			return nil, fmt.Errorf("list messages: %w", err)
		}
		for _, m := range resp.Messages {
			allIDs = append(allIDs, m.Id)
		}
		remaining -= int64(len(resp.Messages))
		pageToken = resp.NextPageToken
		if pageToken == "" || len(resp.Messages) == 0 {
			break
		}
	}

	if len(allIDs) == 0 {
		return nil, nil
	}

	return c.fetchMessagesByIDs(ctx, allIDs, "metadata")
}

// FetchMessagesByIDs fetches full message data for the given message IDs concurrently.
func (c *GmailAPIClient) FetchMessagesByIDs(ctx context.Context, ids []string) ([]*GmailMessage, error) {
	return c.fetchMessagesByIDs(ctx, ids, "full")
}

func (c *GmailAPIClient) fetchMessagesByIDs(ctx context.Context, ids []string, format string) ([]*GmailMessage, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(10)

	messages := make([]*GmailMessage, len(ids))
	for i, id := range ids {
		g.Go(func() error {
			msg, err := c.service.Users.Messages.Get("me", id).Format(format).Context(gctx).Do()
			if err != nil {
				return nil
			}
			messages[i] = convertGmailMessage(msg)
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	var result []*GmailMessage
	for _, m := range messages {
		if m != nil {
			result = append(result, m)
		}
	}
	return result, nil
}

// FetchThread fetches all messages in a Gmail thread.
func (c *GmailAPIClient) FetchThread(ctx context.Context, threadID string) ([]*GmailMessage, error) {
	thread, err := c.service.Users.Threads.Get("me", threadID).Format("full").Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("get thread: %w", err)
	}
	var messages []*GmailMessage
	for _, msg := range thread.Messages {
		messages = append(messages, convertGmailMessage(msg))
	}
	return messages, nil
}

// GetProfile returns the authenticated user's email address.
func (c *GmailAPIClient) GetProfile(ctx context.Context) (string, error) {
	profile, err := c.service.Users.GetProfile("me").Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("get profile: %w", err)
	}
	return profile.EmailAddress, nil
}

// GetLatestHistoryId returns the current historyId for incremental sync.
func (c *GmailAPIClient) GetLatestHistoryId(ctx context.Context) (uint64, error) {
	profile, err := c.service.Users.GetProfile("me").Context(ctx).Do()
	if err != nil {
		return 0, fmt.Errorf("get profile: %w", err)
	}
	return profile.HistoryId, nil
}

// FetchHistory uses the Gmail History API to get message IDs added since the given historyId.
func (c *GmailAPIClient) FetchHistory(ctx context.Context, startHistoryId uint64) (*HistoryResult, error) {
	result := &HistoryResult{}
	seen := make(map[string]bool)
	var pageToken string

	for {
		call := c.service.Users.History.List("me").
			StartHistoryId(startHistoryId).
			HistoryTypes("messageAdded")
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Context(ctx).Do()
		if err != nil {
			if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "notFound") {
				return nil, ErrHistoryExpired
			}
			return nil, fmt.Errorf("list history: %w", err)
		}
		result.LatestHistoryId = resp.HistoryId
		for _, h := range resp.History {
			for _, added := range h.MessagesAdded {
				if added.Message != nil && !seen[added.Message.Id] {
					seen[added.Message.Id] = true
					result.NewMessageIDs = append(result.NewMessageIDs, added.Message.Id)
				}
			}
		}
		pageToken = resp.NextPageToken
		if pageToken == "" {
			break
		}
	}

	return result, nil
}

const gmailBatchLimit = 1000

// ArchiveMessages removes the INBOX label from messages.
func (c *GmailAPIClient) ArchiveMessages(ctx context.Context, ids []string) error {
	return c.batchModifyMessages(ctx, ids, nil, []string{"INBOX"})
}

// UnarchiveMessages adds the INBOX label back to messages.
func (c *GmailAPIClient) UnarchiveMessages(ctx context.Context, ids []string) error {
	return c.batchModifyMessages(ctx, ids, []string{"INBOX"}, nil)
}

func (c *GmailAPIClient) batchModifyMessages(ctx context.Context, ids []string, addLabels, removeLabels []string) error {
	for i := 0; i < len(ids); i += gmailBatchLimit {
		end := i + gmailBatchLimit
		if end > len(ids) {
			end = len(ids)
		}
		req := &gmail.BatchModifyMessagesRequest{
			Ids:            ids[i:end],
			AddLabelIds:    addLabels,
			RemoveLabelIds: removeLabels,
		}
		if err := c.service.Users.Messages.BatchModify("me", req).Context(ctx).Do(); err != nil {
			return fmt.Errorf("batch modify (chunk %d-%d): %w", i, end-1, err)
		}
	}
	return nil
}

// ListDrafts returns a list of drafts with subject/to metadata.
func (c *GmailAPIClient) ListDrafts(ctx context.Context, maxResults int64) ([]GmailDraft, error) {
	if maxResults <= 0 {
		maxResults = 10
	}
	if maxResults > 100 {
		maxResults = 100
	}

	resp, err := c.service.Users.Drafts.List("me").MaxResults(maxResults).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("list drafts: %w", err)
	}

	var drafts []GmailDraft
	for _, d := range resp.Drafts {
		draft := GmailDraft{ID: d.Id}
		if d.Message != nil {
			draft.MessageID = d.Message.Id
			draft.ThreadID = d.Message.ThreadId
		}
		full, err := c.service.Users.Drafts.Get("me", d.Id).Format("metadata").Context(ctx).Do()
		if err == nil && full.Message != nil && full.Message.Payload != nil {
			for _, h := range full.Message.Payload.Headers {
				switch strings.ToLower(h.Name) {
				case "subject":
					draft.Subject = h.Value
				case "to":
					draft.To = h.Value
				}
			}
		}
		drafts = append(drafts, draft)
	}
	return drafts, nil
}

// CreateDraft creates a new Gmail draft.
func (c *GmailAPIClient) CreateDraft(ctx context.Context, to, subject, body string) (*GmailDraft, error) {
	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body)

	raw := base64.URLEncoding.EncodeToString([]byte(msg.String()))

	draft, err := c.service.Users.Drafts.Create("me", &gmail.Draft{
		Message: &gmail.Message{Raw: raw},
	}).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("create draft: %w", err)
	}

	result := &GmailDraft{ID: draft.Id, Subject: subject, To: to}
	if draft.Message != nil {
		result.MessageID = draft.Message.Id
		result.ThreadID = draft.Message.ThreadId
	}
	return result, nil
}

// SendDraft sends a draft by its draft ID.
func (c *GmailAPIClient) SendDraft(ctx context.Context, draftID string) (*GmailDraft, error) {
	msg, err := c.service.Users.Drafts.Send("me", &gmail.Draft{Id: draftID}).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("send draft: %w", err)
	}
	return &GmailDraft{ID: draftID, MessageID: msg.Id, ThreadID: msg.ThreadId}, nil
}

// DeleteDraft permanently deletes a draft.
func (c *GmailAPIClient) DeleteDraft(ctx context.Context, draftID string) error {
	return c.service.Users.Drafts.Delete("me", draftID).Context(ctx).Do()
}

// ReplyToThread sends a reply to an existing Gmail thread.
func (c *GmailAPIClient) ReplyToThread(ctx context.Context, threadID, to, body string) (*GmailDraft, error) {
	thread, err := c.service.Users.Threads.Get("me", threadID).Format("metadata").MetadataHeaders("Message-Id", "Subject").Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("get thread for reply: %w", err)
	}
	if len(thread.Messages) == 0 {
		return nil, fmt.Errorf("thread %s has no messages", threadID)
	}

	lastMsg := thread.Messages[len(thread.Messages)-1]
	var messageID, subject string
	if lastMsg.Payload != nil {
		for _, h := range lastMsg.Payload.Headers {
			switch h.Name {
			case "Message-Id":
				messageID = h.Value
			case "Subject":
				subject = h.Value
			}
		}
	}
	if subject != "" && !strings.HasPrefix(strings.ToLower(subject), "re:") {
		subject = "Re: " + subject
	}

	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	if messageID != "" {
		msg.WriteString(fmt.Sprintf("In-Reply-To: %s\r\n", messageID))
		msg.WriteString(fmt.Sprintf("References: %s\r\n", messageID))
	}
	msg.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body)

	raw := base64.URLEncoding.EncodeToString([]byte(msg.String()))

	sent, err := c.service.Users.Messages.Send("me", &gmail.Message{
		Raw:      raw,
		ThreadId: threadID,
	}).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("send reply: %w", err)
	}

	return &GmailDraft{MessageID: sent.Id, ThreadID: sent.ThreadId, Subject: subject, To: to}, nil
}

// ListAttachments returns metadata for all attachments on a message.
func (c *GmailAPIClient) ListAttachments(ctx context.Context, messageID string) ([]GmailAttachment, error) {
	msg, err := c.service.Users.Messages.Get("me", messageID).Format("full").Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("get message: %w", err)
	}
	var attachments []GmailAttachment
	walkParts(msg.Payload, messageID, &attachments)
	return attachments, nil
}

// GetAttachment downloads an attachment and returns the decoded bytes.
func (c *GmailAPIClient) GetAttachment(ctx context.Context, messageID, attachmentID string) ([]byte, error) {
	att, err := c.service.Users.Messages.Attachments.Get("me", messageID, attachmentID).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("get attachment: %w", err)
	}
	data, err := base64.URLEncoding.DecodeString(att.Data)
	if err != nil {
		return nil, fmt.Errorf("decode attachment: %w", err)
	}
	return data, nil
}

func convertGmailMessage(msg *gmail.Message) *GmailMessage {
	m := &GmailMessage{
		ID:        msg.Id,
		ThreadID:  msg.ThreadId,
		Snippet:   msg.Snippet,
		Labels:    strings.Join(msg.LabelIds, ","),
		IsRead:    !slices.Contains(msg.LabelIds, "UNREAD"),
		FetchedAt: time.Now(),
	}

	if msg.Payload != nil {
		for _, h := range msg.Payload.Headers {
			switch strings.ToLower(h.Name) {
			case "subject":
				m.Subject = h.Value
			case "from":
				m.From = h.Value
			case "to":
				m.To = h.Value
			case "date":
				if t, err := parseEmailDate(h.Value); err == nil {
					m.Date = t
				}
			}
		}
	}

	if m.Date.IsZero() && msg.InternalDate > 0 {
		m.Date = time.Unix(0, msg.InternalDate*int64(time.Millisecond))
	}

	if msg.Payload != nil {
		m.Body = extractBody(msg.Payload)
		if len(m.Body) > 100000 {
			m.Body = m.Body[:100000] + "\n...[truncated]"
		}
	}

	return m
}

func extractBody(payload *gmail.MessagePart) string {
	if payload.Body != nil && payload.Body.Data != "" {
		if strings.HasPrefix(payload.MimeType, "text/") {
			if decoded, err := base64.URLEncoding.DecodeString(payload.Body.Data); err == nil {
				return string(decoded)
			}
		}
	}

	var plainText, htmlText string
	for _, part := range payload.Parts {
		if part.MimeType == "text/plain" && part.Body != nil && part.Body.Data != "" {
			if decoded, err := base64.URLEncoding.DecodeString(part.Body.Data); err == nil {
				plainText = string(decoded)
			}
		} else if part.MimeType == "text/html" && part.Body != nil && part.Body.Data != "" {
			if decoded, err := base64.URLEncoding.DecodeString(part.Body.Data); err == nil {
				htmlText = string(decoded)
			}
		} else if strings.HasPrefix(part.MimeType, "multipart/") {
			nested := extractBody(part)
			if nested != "" && plainText == "" {
				plainText = nested
			}
		}
	}

	if plainText != "" {
		return plainText
	}
	return htmlText
}

func walkParts(part *gmail.MessagePart, messageID string, out *[]GmailAttachment) {
	if part.Filename != "" && part.Body != nil && part.Body.AttachmentId != "" {
		*out = append(*out, GmailAttachment{
			MessageID: messageID,
			PartID:    part.Body.AttachmentId,
			Filename:  part.Filename,
			MimeType:  part.MimeType,
			Size:      part.Body.Size,
		})
	}
	for _, child := range part.Parts {
		walkParts(child, messageID, out)
	}
}

func parseEmailDate(s string) (time.Time, error) {
	formats := []string{
		"Mon, 2 Jan 2006 15:04:05 -0700",
		"Mon, 2 Jan 2006 15:04:05 -0700 (MST)",
		"2 Jan 2006 15:04:05 -0700",
		"Mon, 02 Jan 2006 15:04:05 -0700",
		time.RFC1123Z,
		time.RFC1123,
		time.RFC3339,
		"2006-01-02T15:04:05Z",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unable to parse email date: %s", s)
}
