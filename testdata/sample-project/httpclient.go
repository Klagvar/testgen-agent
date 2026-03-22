// Tests pattern: http.Client.Do (outgoing HTTP requests)
package sample

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// APIResponse represents a generic API response.
type APIResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// FetchStatus makes a GET request to the given URL and returns the status.
func FetchStatus(client *http.Client, url string) (int, error) {
	resp, err := client.Get(url)
	if err != nil {
		return 0, fmt.Errorf("get %s: %w", url, err)
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

// FetchJSON makes a GET request and decodes the JSON response body.
func FetchJSON(client *http.Client, url string) (*APIResponse, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var result APIResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	return &result, nil
}
