package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestPin_BestImageURL(t *testing.T) {
	tests := []struct {
		name     string
		pin      Pin
		expected string
	}{
		{
			name: "Prefers originals over all others",
			pin: Pin{
				Media: &PinMedia{
					Images: map[string]ImageSpec{
						"150x150":   {URL: "https://example.com/150.jpg"},
						"600x":      {URL: "https://example.com/600.jpg"},
						"1200x":     {URL: "https://example.com/1200.jpg"},
						"originals": {URL: "https://example.com/orig.jpg"},
					},
				},
			},
			expected: "https://example.com/orig.jpg",
		},
		{
			name: "Falls back to 1200x when originals missing",
			pin: Pin{
				Media: &PinMedia{
					Images: map[string]ImageSpec{
						"150x150": {URL: "https://example.com/150.jpg"},
						"600x":    {URL: "https://example.com/600.jpg"},
						"1200x":   {URL: "https://example.com/1200.jpg"},
					},
				},
			},
			expected: "https://example.com/1200.jpg",
		},
		{
			name: "Falls back to Pin.Images root field",
			pin: Pin{
				Images: map[string]ImageSpec{
					"600x": {URL: "https://example.com/pin-images-600.jpg"},
				},
			},
			expected: "https://example.com/pin-images-600.jpg",
		},
		{
			name: "Returns empty string when no images found",
			pin: Pin{
				Media: &PinMedia{
					Images: map[string]ImageSpec{},
				},
			},
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.pin.BestImageURL()
			if got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

func TestPin_Filename(t *testing.T) {
	tests := []struct {
		name     string
		pin      Pin
		index    int
		expected string
	}{
		{
			name: "Uses pin ID and jpg extension",
			pin: Pin{
				ID: "123456789",
				Images: map[string]ImageSpec{
					"originals": {URL: "https://i.pinimg.com/originals/ab/cd/ef/photo.jpg?query=1"},
				},
			},
			index:    0,
			expected: "pin_123456789.jpg",
		},
		{
			name: "Preserves png extension",
			pin: Pin{
				ID: "987654321",
				Images: map[string]ImageSpec{
					"originals": {URL: "https://i.pinimg.com/originals/ab/cd/ef/graphic.PNG"},
				},
			},
			index:    1,
			expected: "pin_987654321.png",
		},
		{
			name: "Preserves webp extension",
			pin: Pin{
				ID: "456",
				Images: map[string]ImageSpec{
					"originals": {URL: "https://i.pinimg.com/originals/ab/cd/ef/art.webp"},
				},
			},
			index:    2,
			expected: "pin_456.webp",
		},
		{
			name: "Falls back to index and default jpg when no ID",
			pin: Pin{
				Images: map[string]ImageSpec{
					"originals": {URL: "https://i.pinimg.com/unknown-format"},
				},
			},
			index:    5,
			expected: "pin_5.jpg",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.pin.Filename(tc.index)
			if got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

func TestPinterestClient_GetBoardPins_Pagination(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-api-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if r.URL.Path != "/boards/test-board/pins" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		bookmark := r.URL.Query().Get("bookmark")

		if bookmark == "" {
			// First page
			nextBookmark := "page2_cursor"
			resp := BoardPinsResponse{
				Items: []Pin{
					{ID: "pin-1", Title: "Pin 1"},
					{ID: "pin-2", Title: "Pin 2"},
				},
				Bookmark: &nextBookmark,
			}
			json.NewEncoder(w).Encode(resp)
		} else if bookmark == "page2_cursor" {
			// Second page (final)
			resp := BoardPinsResponse{
				Items: []Pin{
					{ID: "pin-3", Title: "Pin 3"},
				},
				Bookmark: nil,
			}
			json.NewEncoder(w).Encode(resp)
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()

	client := NewPinterestClientWithBaseURL("test-api-key", server.URL)
	pins, err := client.GetBoardPins("test-board")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pins) != 3 {
		t.Fatalf("expected 3 pins across 2 pages, got %d", len(pins))
	}
	if pins[0].ID != "pin-1" || pins[1].ID != "pin-2" || pins[2].ID != "pin-3" {
		t.Errorf("pins content mismatch: %+v", pins)
	}
	if requestCount != 2 {
		t.Errorf("expected 2 HTTP requests for pagination, got %d", requestCount)
	}
}

func TestPinterestClient_GetBoardPins_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		resp := BoardPinsResponse{
			Code:    2,
			Message: "The requested board_id could not be found.",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewPinterestClientWithBaseURL("test-api-key", server.URL)
	pins, err := client.GetBoardPins("nonexistent-board")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if pins != nil {
		t.Errorf("expected pins to be nil, got %+v", pins)
	}
}

func TestPinterestClient_GetBoards(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/boards" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		resp := BoardsResponse{
			Items: []Board{
				{ID: "b-1", Name: "Inspirations", PinCount: 42},
				{ID: "b-2", Name: "Recipes", PinCount: 15},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewPinterestClientWithBaseURL("test-api-key", server.URL)
	boards, err := client.GetBoards()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(boards) != 2 {
		t.Fatalf("expected 2 boards, got %d", len(boards))
	}
	if boards[0].Name != "Inspirations" || boards[1].PinCount != 15 {
		t.Errorf("board data mismatch: %+v", boards)
	}
}

func TestPinterestClient_DownloadImage(t *testing.T) {
	testData := []byte("fake-image-bytes-data-12345")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/image.jpg" {
			w.WriteHeader(http.StatusOK)
			w.Write(testData)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	tempDir := t.TempDir()
	client := NewPinterestClientWithBaseURL("test-key", server.URL)

	downloaded, err := client.DownloadImage(server.URL+"/image.jpg", tempDir, "test.jpg")
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}
	if !downloaded {
		t.Errorf("expected downloaded=true for first download")
	}

	content, err := os.ReadFile(filepath.Join(tempDir, "test.jpg"))
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}
	if string(content) != string(testData) {
		t.Errorf("expected content %q, got %q", string(testData), string(content))
	}

	downloaded, err = client.DownloadImage(server.URL+"/image.jpg", tempDir, "test.jpg")
	if err != nil {
		t.Fatalf("unexpected error on second download: %v", err)
	}
	if downloaded {
		t.Errorf("expected downloaded=false when file already exists")
	}

	_, err = client.DownloadImage(server.URL+"/not-found.jpg", tempDir, "fail.jpg")
	if err == nil {
		t.Errorf("expected error for 404 image, got nil")
	}
	if _, statErr := os.Stat(filepath.Join(tempDir, "fail.jpg")); statErr == nil {
		t.Errorf("fail.jpg should not exist after failed download")
	}
	if _, statErr := os.Stat(filepath.Join(tempDir, "fail.jpg.tmp")); statErr == nil {
		t.Errorf("fail.jpg.tmp should be cleaned up after failed download")
	}
}
