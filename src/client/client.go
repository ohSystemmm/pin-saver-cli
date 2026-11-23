package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

const (
	DefaultBaseURL = "https://api.pinterest.com/v5/"
)

type ImageSpec struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type PinMedia struct {
	MediaType string               `json:"media_type"`
	Images    map[string]ImageSpec `json:"images"`
}

type Pin struct {
	ID          string               `json:"id"`
	Title       string               `json:"title"`
	Description string               `json:"description"`
	Link        string               `json:"link"`
	Media       *PinMedia            `json:"media"`
	Images      map[string]ImageSpec `json:"images"`
}

func (p *Pin) BestImageURL() string {
	candidates := []map[string]ImageSpec{}
	if p.Media != nil && len(p.Media.Images) > 0 {
		candidates = append(candidates, p.Media.Images)
	}
	if len(p.Images) > 0 {
		candidates = append(candidates, p.Images)
	}

	preferredKeys := []string{"originals", "1200x", "736x", "600x", "400x300", "150x150"}
	for _, images := range candidates {
		for _, key := range preferredKeys {
			if spec, ok := images[key]; ok && spec.URL != "" {
				return spec.URL
			}
		}
		for _, spec := range images {
			if spec.URL != "" {
				return spec.URL
			}
		}
	}
	return ""
}

   func (p *Pin) Filename(index int) string {
	ext := ".jpg"
	imageURL := p.BestImageURL()
	if parsed, err := url.Parse(imageURL); err == nil {
		pathExt := strings.ToLower(filepath.Ext(parsed.Path))
		if pathExt == ".jpg" || pathExt == ".jpeg" || pathExt == ".png" || pathExt == ".webp" || pathExt == ".gif" {
			ext = pathExt
		}
	}
	if p.ID != "" {
		return fmt.Sprintf("pin_%s%s", p.ID, ext)
	}
	return fmt.Sprintf("pin_%d%s", index, ext)
}

type Board struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Privacy     string `json:"privacy"`
	PinCount    int    `json:"pin_count"`
}

type BoardPinsResponse struct {
	Items    []Pin   `json:"items"`
	Bookmark *string `json:"bookmark"`
	Code     int     `json:"code,omitempty"`
	Message  string  `json:"message,omitempty"`
}

type BoardsResponse struct {
	Items    []Board `json:"items"`
	Bookmark *string `json:"bookmark"`
	Code     int     `json:"code,omitempty"`
	Message  string  `json:"message,omitempty"`
}

type PinterestClient struct {
	apiKey     string
	baseURL    string
	restClient *resty.Client
	httpClient *http.Client
}

func NewPinterestClient(apiKey string) *PinterestClient {
	return NewPinterestClientWithBaseURL(apiKey, DefaultBaseURL)
}

func NewPinterestClientWithBaseURL(apiKey, baseURL string) *PinterestClient {
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	restClient := resty.New().
		SetTimeout(30 * time.Second).
		SetRetryCount(2).
		SetRetryWaitTime(1 * time.Second).
		SetRetryMaxWaitTime(5 * time.Second)

	httpClient := &http.Client{
		Timeout: 60 * time.Second,
	}

	return &PinterestClient{
		apiKey:     apiKey,
		baseURL:    baseURL,
		restClient: restClient,
		httpClient: httpClient,
	}
}

func (p *PinterestClient) GetBoardPins(boardID string) ([]Pin, error) {
	if strings.TrimSpace(boardID) == "" {
		return nil, fmt.Errorf("board ID cannot be empty")
	}

	var allPins []Pin
	bookmark := ""

	for {
		req := p.restClient.R().
			SetHeader("Authorization", "Bearer "+p.apiKey).
			SetQueryParam("page_size", "100")

		if bookmark != "" {
			req.SetQueryParam("bookmark", bookmark)
		}

		var respData BoardPinsResponse
		resp, err := req.
			SetResult(&respData).
			Get(fmt.Sprintf("%sboards/%s/pins", p.baseURL, boardID))

		if err != nil {
			return nil, fmt.Errorf("failed to request pins: %w", err)
		}

		if resp.IsError() {
			if respData.Message != "" {
				return nil, fmt.Errorf("error fetching pins (HTTP %d, code %d): %s", resp.StatusCode(), respData.Code, respData.Message)
			}
			return nil, fmt.Errorf("error fetching pins: HTTP %d: %s", resp.StatusCode(), resp.String())
		}

		allPins = append(allPins, respData.Items...)

		if respData.Bookmark == nil || *respData.Bookmark == "" || *respData.Bookmark == bookmark {
			break
		}
		bookmark = *respData.Bookmark
	}

	return allPins, nil
}

func (p *PinterestClient) GetBoards() ([]Board, error) {
	var allBoards []Board
	bookmark := ""

	for {
		req := p.restClient.R().
			SetHeader("Authorization", "Bearer "+p.apiKey).
			SetQueryParam("page_size", "50")

		if bookmark != "" {
			req.SetQueryParam("bookmark", bookmark)
		}

		var respData BoardsResponse
		resp, err := req.
			SetResult(&respData).
			Get(fmt.Sprintf("%sboards", p.baseURL))

		if err != nil {
			return nil, fmt.Errorf("failed to request boards: %w", err)
		}

		if resp.IsError() {
			if respData.Message != "" {
				return nil, fmt.Errorf("error fetching boards (HTTP %d, code %d): %s", resp.StatusCode(), respData.Code, respData.Message)
			}
			return nil, fmt.Errorf("error fetching boards: HTTP %d: %s", resp.StatusCode(), resp.String())
		}

		allBoards = append(allBoards, respData.Items...)

		if respData.Bookmark == nil || *respData.Bookmark == "" || *respData.Bookmark == bookmark {
			break
		}
		bookmark = *respData.Bookmark
	}

	return allBoards, nil
}

func (p *PinterestClient) DownloadImage(rawURL, directory, filename string) (bool, error) {
	if rawURL == "" {
		return false, fmt.Errorf("image URL cannot be empty")
	}

	if err := os.MkdirAll(directory, 0755); err != nil {
		return false, fmt.Errorf("failed to create directory %q: %w", directory, err)
	}

	filePath := filepath.Join(directory, filename)

	if fi, err := os.Stat(filePath); err == nil && fi.Size() > 0 {
		return false, nil
	}

	resp, err := p.httpClient.Get(rawURL)
	if err != nil {
		return false, fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("bad status %d (%s) while downloading %s", resp.StatusCode, resp.Status, rawURL)
	}

	tmpPath := filePath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return false, fmt.Errorf("failed to create temp file %q: %w", tmpPath, err)
	}

	success := false
	defer func() {
		out.Close()
		if !success {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err = io.Copy(out, resp.Body); err != nil {
		return false, fmt.Errorf("failed to write image content: %w", err)
	}

	if err := out.Close(); err != nil {
		return false, fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		return false, fmt.Errorf("failed to save final file %q: %w", filePath, err)
	}

	success = true
	return true, nil
}