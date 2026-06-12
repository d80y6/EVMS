package evms

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL:    baseURL,
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) do(method, path string, body, result interface{}) error {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req, err := http.NewRequest(method, c.baseURL+path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}
	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}
	return nil
}

func (c *Client) ListCameras() ([]map[string]interface{}, error) {
	var result struct {
		Cameras []map[string]interface{} `json:"cameras"`
	}
	err := c.do("GET", "/api/cameras", nil, &result)
	return result.Cameras, err
}

func (c *Client) GetEvents(cameraID string, since time.Time) ([]map[string]interface{}, error) {
	var result struct {
		Events []map[string]interface{} `json:"events"`
	}
	path := fmt.Sprintf("/api/events?camera_id=%s&start_time=%s", cameraID, since.Format(time.RFC3339))
	err := c.do("GET", path, nil, &result)
	return result.Events, err
}

func (c *Client) SendPTZCommand(cameraID, command string) error {
	return c.do("POST", fmt.Sprintf("/api/cameras/%s/ptz/%s", cameraID, command), nil, nil)
}

func (c *Client) SearchRecordings(cameraID string, start, end time.Time) ([]map[string]interface{}, error) {
	var result struct {
		Recordings []map[string]interface{} `json:"recordings"`
	}
	path := fmt.Sprintf("/api/recordings?camera_id=%s&start_time=%s&end_time=%s",
		cameraID, start.Format(time.RFC3339), end.Format(time.RFC3339))
	err := c.do("GET", path, nil, &result)
	return result.Recordings, err
}
