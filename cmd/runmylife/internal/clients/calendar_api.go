package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

// CalendarAPIClient wraps Google's Calendar API for live calendar access.
type CalendarAPIClient struct {
	service *calendar.Service
	Account string
}

// CalendarEvent represents a calendar event with extracted fields.
type CalendarEvent struct {
	ID             string
	Summary        string
	Description    string
	Location       string
	StartTime      time.Time
	EndTime        time.Time
	Attendees      string // JSON array of emails
	IsRecurring    bool
	CalendarID     string
	Status         string
	Organizer      string
	ConferenceLink string
	ResponseStatus string
	FetchedAt      time.Time
}

// CalendarSummary holds basic calendar info from ListCalendars.
type CalendarSummary struct {
	ID          string `json:"id"`
	Summary     string `json:"summary"`
	Primary     bool   `json:"primary"`
	AccessRole  string `json:"access_role"`
	Description string `json:"description,omitempty"`
}

// AvailabilitySlot represents a free or busy time block.
type AvailabilitySlot struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
	Busy  bool      `json:"busy"`
}

// NewCalendarAPIClient creates a client for the given account.
func NewCalendarAPIClient(ctx context.Context, account string) (*CalendarAPIClient, error) {
	credPath := GoogleCredentialsPathForAccount(account)

	if account != "" && account != "personal" {
		if _, err := os.Stat(credPath); os.IsNotExist(err) {
			credPath = GoogleCredentialsPathForAccount("")
		}
	}

	oauthConfig, err := LoadGoogleCredentials(credPath, DefaultCalendarScopes()...)
	if err != nil {
		return nil, fmt.Errorf("load credentials for calendar: %w", err)
	}

	token, err := LoadGoogleTokenForAccount(account)
	if err != nil {
		return nil, err
	}

	ts := oauthConfig.TokenSource(ctx, token)
	savingTS := &savingTokenSource{base: ts, lastToken: token, account: account}

	svc, err := calendar.NewService(ctx, option.WithTokenSource(savingTS))
	if err != nil {
		return nil, fmt.Errorf("create calendar service: %w", err)
	}

	acctName := account
	if acctName == "" {
		acctName = "personal"
	}
	return &CalendarAPIClient{service: svc, Account: acctName}, nil
}

// FetchEvents retrieves events from the primary calendar in the given time range.
func (c *CalendarAPIClient) FetchEvents(ctx context.Context, timeMin, timeMax time.Time, maxResults int64) ([]*CalendarEvent, error) {
	if maxResults <= 0 {
		maxResults = 50
	}
	if maxResults > 250 {
		maxResults = 250
	}

	call := c.service.Events.List("primary").
		TimeMin(timeMin.Format(time.RFC3339)).
		TimeMax(timeMax.Format(time.RFC3339)).
		MaxResults(maxResults).
		SingleEvents(true).
		OrderBy("startTime")

	resp, err := call.Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("list calendar events: %w", err)
	}

	var events []*CalendarEvent
	for _, item := range resp.Items {
		if ev := convertCalendarEvent(item, c.Account); ev != nil {
			events = append(events, ev)
		}
	}
	return events, nil
}

// ListCalendars returns a list of calendars the user has access to.
func (c *CalendarAPIClient) ListCalendars(ctx context.Context) ([]CalendarSummary, error) {
	resp, err := c.service.CalendarList.List().Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("list calendars: %w", err)
	}

	var cals []CalendarSummary
	for _, item := range resp.Items {
		cals = append(cals, CalendarSummary{
			ID:          item.Id,
			Summary:     item.Summary,
			Primary:     item.Primary,
			AccessRole:  item.AccessRole,
			Description: item.Description,
		})
	}
	return cals, nil
}

// CreateEvent creates a new event on the specified calendar.
func (c *CalendarAPIClient) CreateEvent(ctx context.Context, calendarID, summary, description, location string, startTime, endTime time.Time, attendees []string, addMeet bool) (*CalendarEvent, error) {
	if calendarID == "" {
		calendarID = "primary"
	}

	event := &calendar.Event{
		Summary:     summary,
		Description: description,
		Location:    location,
		Start: &calendar.EventDateTime{
			DateTime: startTime.Format(time.RFC3339),
			TimeZone: startTime.Location().String(),
		},
		End: &calendar.EventDateTime{
			DateTime: endTime.Format(time.RFC3339),
			TimeZone: endTime.Location().String(),
		},
	}

	for _, email := range attendees {
		event.Attendees = append(event.Attendees, &calendar.EventAttendee{Email: email})
	}

	if addMeet {
		event.ConferenceData = &calendar.ConferenceData{
			CreateRequest: &calendar.CreateConferenceRequest{
				RequestId:             fmt.Sprintf("runmylife-%d", time.Now().UnixNano()),
				ConferenceSolutionKey: &calendar.ConferenceSolutionKey{Type: "hangoutsMeet"},
			},
		}
	}

	call := c.service.Events.Insert(calendarID, event)
	if addMeet {
		call = call.ConferenceDataVersion(1)
	}

	created, err := call.Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("create calendar event: %w", err)
	}

	return convertCalendarEvent(created, c.Account), nil
}

// FindAvailability queries the FreeBusy API and returns busy periods.
func (c *CalendarAPIClient) FindAvailability(ctx context.Context, calendarIDs []string, timeMin, timeMax time.Time) ([]AvailabilitySlot, error) {
	if len(calendarIDs) == 0 {
		calendarIDs = []string{"primary"}
	}

	var items []*calendar.FreeBusyRequestItem
	for _, id := range calendarIDs {
		items = append(items, &calendar.FreeBusyRequestItem{Id: id})
	}

	req := &calendar.FreeBusyRequest{
		TimeMin: timeMin.Format(time.RFC3339),
		TimeMax: timeMax.Format(time.RFC3339),
		Items:   items,
	}

	resp, err := c.service.Freebusy.Query(req).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("query freebusy: %w", err)
	}

	var busy []AvailabilitySlot
	for _, cal := range resp.Calendars {
		for _, period := range cal.Busy {
			start, _ := time.Parse(time.RFC3339, period.Start)
			end, _ := time.Parse(time.RFC3339, period.End)
			busy = append(busy, AvailabilitySlot{Start: start, End: end, Busy: true})
		}
	}

	return busy, nil
}

func convertCalendarEvent(item *calendar.Event, account string) *CalendarEvent {
	if item == nil {
		return nil
	}

	startTime := parseEventTime(item.Start)
	endTime := parseEventTime(item.End)

	var attendeeEmails []string
	for _, a := range item.Attendees {
		attendeeEmails = append(attendeeEmails, a.Email)
	}
	attendeesJSON, _ := json.Marshal(attendeeEmails)

	var conferenceLink string
	if item.ConferenceData != nil {
		for _, ep := range item.ConferenceData.EntryPoints {
			if ep.EntryPointType == "video" || ep.Uri != "" {
				conferenceLink = ep.Uri
				break
			}
		}
	}
	if conferenceLink == "" && item.HangoutLink != "" {
		conferenceLink = item.HangoutLink
	}

	var organizer string
	if item.Organizer != nil {
		organizer = item.Organizer.Email
	}

	var responseStatus string
	for _, a := range item.Attendees {
		if a.Self {
			responseStatus = a.ResponseStatus
			break
		}
	}

	return &CalendarEvent{
		ID:             item.Id,
		Summary:        item.Summary,
		Description:    item.Description,
		Location:       item.Location,
		StartTime:      startTime,
		EndTime:        endTime,
		Attendees:      string(attendeesJSON),
		IsRecurring:    item.RecurringEventId != "",
		CalendarID:     "primary",
		Status:         item.Status,
		Organizer:      organizer,
		ConferenceLink: conferenceLink,
		ResponseStatus: responseStatus,
		FetchedAt:      time.Now(),
	}
}

func parseEventTime(edt *calendar.EventDateTime) time.Time {
	if edt == nil {
		return time.Time{}
	}
	if edt.DateTime != "" {
		t, err := time.Parse(time.RFC3339, edt.DateTime)
		if err == nil {
			return t
		}
	}
	if edt.Date != "" {
		t, err := time.Parse("2006-01-02", edt.Date)
		if err == nil {
			return t
		}
	}
	return time.Time{}
}
