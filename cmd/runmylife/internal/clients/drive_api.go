package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const driveAPIBaseURL = "https://www.googleapis.com/drive/v3"

type DriveClient struct {
	httpClient *http.Client
	token      string
}

type DriveFile struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	MimeType   string   `json:"mimeType"`
	Parents    []string `json:"parents,omitempty"`
	Size       int64    `json:"size,string,omitempty"`
	ModifiedAt string   `json:"modifiedTime"`
	Shared     bool     `json:"shared"`
	WebLink    string   `json:"webViewLink"`
}

func NewDriveClient(accessToken string) *DriveClient {
	return &DriveClient{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		token:      accessToken,
	}
}

func (d *DriveClient) ListFiles(ctx context.Context, folderId string, limit int) ([]DriveFile, error) {
	if limit <= 0 {
		limit = 20
	}
	params := url.Values{
		"pageSize": {fmt.Sprintf("%d", limit)},
		"fields":   {"files(id,name,mimeType,parents,size,modifiedTime,shared,webViewLink)"},
		"orderBy":  {"modifiedTime desc"},
	}
	if folderId != "" {
		params.Set("q", fmt.Sprintf("'%s' in parents", folderId))
	}
	return d.listFiles(ctx, params)
}

func (d *DriveClient) SearchFiles(ctx context.Context, query string, limit int) ([]DriveFile, error) {
	if limit <= 0 {
		limit = 20
	}
	params := url.Values{
		"pageSize": {fmt.Sprintf("%d", limit)},
		"fields":   {"files(id,name,mimeType,parents,size,modifiedTime,shared,webViewLink)"},
		"orderBy":  {"modifiedTime desc"},
		"q":        {fmt.Sprintf("name contains '%s'", query)},
	}
	return d.listFiles(ctx, params)
}

func (d *DriveClient) ListFolders(ctx context.Context, limit int) ([]DriveFile, error) {
	if limit <= 0 {
		limit = 20
	}
	params := url.Values{
		"pageSize": {fmt.Sprintf("%d", limit)},
		"fields":   {"files(id,name,mimeType,parents,size,modifiedTime,shared,webViewLink)"},
		"orderBy":  {"modifiedTime desc"},
		"q":        {"mimeType = 'application/vnd.google-apps.folder'"},
	}
	return d.listFiles(ctx, params)
}

func (d *DriveClient) GetFile(ctx context.Context, fileId string) (*DriveFile, error) {
	reqURL := fmt.Sprintf("%s/files/%s?fields=id,name,mimeType,parents,size,modifiedTime,shared,webViewLink", driveAPIBaseURL, fileId)
	var file DriveFile
	if err := d.doGet(ctx, reqURL, &file); err != nil {
		return nil, err
	}
	return &file, nil
}

func (d *DriveClient) listFiles(ctx context.Context, params url.Values) ([]DriveFile, error) {
	reqURL := driveAPIBaseURL + "/files?" + params.Encode()
	var result struct {
		Files []DriveFile `json:"files"`
	}
	if err := d.doGet(ctx, reqURL, &result); err != nil {
		return nil, err
	}
	return result.Files, nil
}

func (d *DriveClient) doGet(ctx context.Context, reqURL string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+d.token)
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("drive API request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body := make([]byte, 1024)
		n, _ := resp.Body.Read(body)
		return &HTTPError{StatusCode: resp.StatusCode, Body: string(body[:n]), API: "drive"}
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
