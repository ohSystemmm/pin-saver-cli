package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

func promptInput(reader *bufio.Reader, promptText, defaultValue string) string {
	if defaultValue != "" {
		fmt.Printf("%s [%s]: ", promptText, defaultValue)
	} else {
		fmt.Printf("%s: ", promptText)
	}

	input, err := reader.ReadString('\n')
	if err != nil {
		return defaultValue
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue
	}
	return input
}

func printBoards(boards []Board) {
	if len(boards) == 0 {
		fmt.Println("No boards found for this account.")
		return
	}
	fmt.Printf("\nFound %d board(s):\n", len(boards))
	fmt.Printf("%-20s %-25s %-10s\n", "Board ID", "Name", "Pins")
	fmt.Println(strings.Repeat("-", 60))
	for _, b := range boards {
		name := b.Name
		if len(name) > 23 {
			name = name[:20] + "..."
		}
		fmt.Printf("%-20s %-25s %-10d\n", b.ID, name, b.PinCount)
	}
	fmt.Println()
}

func main() {
	tokenFlag := flag.String("token", "", "Pinterest API key / access token (or set PINTEREST_ACCESS_TOKEN)")
	boardFlag := flag.String("board", "", "Pinterest board ID (numeric string)")
	dirFlag := flag.String("dir", "", "Directory where downloaded images are stored (default: ./downloads)")
	listFlag := flag.Bool("list-boards", false, "List available Pinterest boards for the user and exit")
	concurrencyFlag := flag.Int("concurrency", 5, "Number of concurrent image downloads")
	flag.Parse()

	reader := bufio.NewReader(os.Stdin)

	apiKey := *tokenFlag
	if apiKey == "" {
		apiKey = os.Getenv("PINTEREST_ACCESS_TOKEN")
		if apiKey == "" {
			apiKey = os.Getenv("PINTEREST_API_KEY")
		}
	}
	if apiKey == "" {
		apiKey = promptInput(reader, "Enter your Pinterest API key / access token", "")
		if apiKey == "" {
			log.Fatal("Error: Pinterest API key / access token is required.")
		}
	}

	client := NewPinterestClient(apiKey)

	if *listFlag {
		fmt.Println("Fetching your Pinterest boards...")
		boards, err := client.GetBoards()
		if err != nil {
			log.Fatalf("Failed to fetch boards: %v", err)
		}
		printBoards(boards)
		return
	}

	boardID := strings.TrimSpace(*boardFlag)
	if boardID == "" {
		for {
			boardID = promptInput(reader, "Enter Pinterest board ID (or type 'list' to view your boards)", "")
			if strings.EqualFold(boardID, "list") {
				fmt.Println("Fetching your Pinterest boards...")
				boards, err := client.GetBoards()
				if err != nil {
					log.Printf("Failed to fetch boards: %v", err)
				} else {
					printBoards(boards)
				}
				continue
			}
			break
		}
		if boardID == "" {
			log.Fatal("Error: Board ID is required.")
		}
	}

	directory := *dirFlag
	if directory == "" {
		directory = promptInput(reader, "Enter directory to save images", "./downloads")
	}
	directory = filepath.Clean(directory)

	if err := os.MkdirAll(directory, 0755); err != nil {
		log.Fatalf("Failed to create directory %q: %v", directory, err)
	}

	fmt.Printf("Fetching pins for board %s...\n", boardID)
	pins, err := client.GetBoardPins(boardID)
	if err != nil {
		log.Fatalf("Failed to get pins: %v", err)
	}

	totalPins := len(pins)
	if totalPins == 0 {
		fmt.Println("No pins found on this board.")
		return
	}

	fmt.Printf("Found %d pin(s). Starting download to %s (concurrency: %d)...\n\n", totalPins, directory, *concurrencyFlag)

	concurrency := *concurrencyFlag
	if concurrency < 1 {
		concurrency = 1
	}

	type downloadTask struct {
		index int
		pin   Pin
	}

	tasks := make(chan downloadTask, totalPins)
	for i, pin := range pins {
		tasks <- downloadTask{index: i, pin: pin}
	}
	close(tasks)

	var (
		downloaded int64
		skipped    int64
		failed     int64
		wg         sync.WaitGroup
	)

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range tasks {
				imgURL := task.pin.BestImageURL()
				filename := task.pin.Filename(task.index)

				if imgURL == "" {
					atomic.AddInt64(&failed, 1)
					log.Printf("[FAILED] Pin %s: no image URL found\n", task.pin.ID)
					continue
				}

				wasDownloaded, err := client.DownloadImage(imgURL, directory, filename)
				if err != nil {
					atomic.AddInt64(&failed, 1)
					log.Printf("[FAILED] Pin %s (%s): %v\n", task.pin.ID, filename, err)
				} else if wasDownloaded {
					atomic.AddInt64(&downloaded, 1)
					fmt.Printf("[OK] Downloaded %s\n", filepath.Join(directory, filename))
				} else {
					atomic.AddInt64(&skipped, 1)
					fmt.Printf("[SKIP] %s (already exists)\n", filepath.Join(directory, filename))
				}
			}
		}()
	}

	wg.Wait()

	fmt.Printf("\nDownload complete!\n")
	fmt.Printf("Total pins: %d | Downloaded: %d | Skipped: %d | Failed: %d\n",
		totalPins, downloaded, skipped, failed)
}