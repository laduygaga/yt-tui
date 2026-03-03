package youtube

import (
	"fmt"
	"os/exec"
	"time"
)

type Video struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Channel     string `json:"channel"`
	Duration    string `json:"duration"`
	Views       string `json:"view_count"`
	Uploaded    string `json:"upload_date"`
	Thumbnail   string `json:"thumbnail"`
	Description string `json:"description"`
	URL         string `json:"url"`
}

type SearchResult struct {
	Videos []Video
}

func Search(query string, maxResults int) ([]Video, error) {
	args := []string{
		"--no-check-certificate",
		"--flat-playlist",
		"--print", "%(id)s|%(title)s|%(channel)s|%(duration)s|%(view_count)s|%(upload_date)s|%(thumbnail)s|%(description)s|%(url)s",
		"--", fmt.Sprintf("ytsearch%d:%s", maxResults, query),
	}

	cmd := exec.Command("yt-dlp", args...)
	output, err := cmd.Output()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("yt-dlp error: %s", string(exitError.Stderr))
		}
		return nil, fmt.Errorf("yt-dlp error: %w", err)
	}

	var videos []Video
	lines := splitLines(string(output))
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := splitPipe(line)
		if len(parts) >= 9 {
			videos = append(videos, Video{
				ID:          parts[0],
				Title:       parts[1],
				Channel:     parts[2],
				Duration:    parts[3],
				Views:       parts[4],
				Uploaded:    parts[5],
				Thumbnail:   parts[6],
				Description: parts[7],
				URL:         parts[8],
			})
		}
	}

	return videos, nil
}

func GetVideoInfo(videoID string) (*Video, error) {
	args := []string{
		"--print", "%(id)s|%(title)s|%(channel)s|%(duration)s|%(view_count)s|%(upload_date)s|%(thumbnail)s|%(description)s|%(url)s",
		"--", videoID,
	}

	cmd := exec.Command("yt-dlp", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp error: %w", err)
	}

	lines := splitLines(string(output))
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := splitPipe(line)
		if len(parts) >= 9 {
			return &Video{
				ID:          parts[0],
				Title:       parts[1],
				Channel:     parts[2],
				Duration:    parts[3],
				Views:       parts[4],
				Uploaded:    parts[5],
				Thumbnail:   parts[6],
				Description: parts[7],
				URL:         parts[8],
			}, nil
		}
	}

	return nil, fmt.Errorf("video not found")
}

func GetStreamURL(videoURL string) (string, error) {
	args := []string{
		"--no-check-certificate",
		"-f", "bestaudio",
		"-g",
		"--", videoURL,
	}

	cmd := exec.Command("yt-dlp", args...)
	output, err := cmd.Output()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("yt-dlp error: %s", string(exitError.Stderr))
		}
		return "", fmt.Errorf("yt-dlp error: %w", err)
	}

	return string(output), nil
}

func Trending() ([]Video, error) {
	return Search("trending", 20)
}

func splitLines(s string) []string {
	var lines []string
	current := ""
	for _, r := range s {
		if r == '\n' {
			lines = append(lines, current)
			current = ""
		} else {
			current += string(r)
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func splitPipe(s string) []string {
	var parts []string
	current := ""
	for _, r := range s {
		if r == '|' {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(r)
		}
	}
	parts = append(parts, current)
	return parts
}

type HistoryItem struct {
	Video     Video
	WatchedAt time.Time
}

func SaveWatchHistory(video Video) error {
	return nil
}
