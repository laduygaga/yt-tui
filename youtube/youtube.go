package youtube

import (
	"fmt"
	"os/exec"
	"strings"
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
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
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

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
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
		"-f", "bestaudio/best",
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

type HistoryItem struct {
	Video     Video
	WatchedAt time.Time
}

func SaveWatchHistory(video Video) error {
	return nil
}
