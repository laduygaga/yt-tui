package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type TranscriptLine struct {
	Text     string  `json:"text"`
	Start    float64 `json:"start"`
	Duration float64 `json:"duration"`
}

type Transcript struct {
	VideoID string           `json:"video_id"`
	Lines   []TranscriptLine `json:"lines"`
	Lang    string           `json:"lang"`
}

type Video struct {
	ID          string      `json:"id"`
	Title       string      `json:"title"`
	Channel     string      `json:"channel"`
	Duration    string      `json:"duration"`
	Views       string      `json:"view_count"`
	Uploaded    string      `json:"upload_date"`
	Thumbnail   string      `json:"thumbnail"`
	Description string      `json:"description"`
	URL         string      `json:"url"`
	Transcript  *Transcript `json:"-"` // Don't serialize to JSON
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

func GetTranscript(videoID string) (*Transcript, error) {
	tmpDir, err := os.MkdirTemp("", "yt-tui-transcript-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	languages := []string{"en", "en-US", "en-GB"}

	for _, lang := range languages {
		args := []string{
			"--write-auto-sub",
			"--sub-lang", lang,
			"--skip-download",
			"--sub-format", "json3",
			"-o", tmpDir + "/%(id)s.%(ext)s",
			"--", videoID,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		cmd := exec.CommandContext(ctx, "yt-dlp", args...)
		cmd.Run()
		cancel()

		subtitlePath := fmt.Sprintf("%s/%s.%s.json3", tmpDir, videoID, lang)
		if data, err := os.ReadFile(subtitlePath); err == nil {
			return parseJSON3Transcript(videoID, data)
		}

		baseLang := strings.Split(lang, "-")[0]
		if baseLang != lang {
			subtitlePath = fmt.Sprintf("%s/%s.%s.json3", tmpDir, videoID, baseLang)
			if data, err := os.ReadFile(subtitlePath); err == nil {
				return parseJSON3Transcript(videoID, data)
			}
		}
	}

	return nil, fmt.Errorf("transcript not available for this video")
}

func parseJSON3Transcript(videoID string, data []byte) (*Transcript, error) {
	var rawTrans struct {
		Events []struct {
			TStart int `json:"tStartMs"`
			Dur    int `json:"dDurationMs"`
			Segs   []struct {
				Text string `json:"utf8"`
			} `json:"segs"`
		} `json:"events"`
	}

	if err := json.Unmarshal(data, &rawTrans); err != nil {
		return nil, fmt.Errorf("failed to parse transcript JSON: %w", err)
	}

	var lines []TranscriptLine
	for _, event := range rawTrans.Events {
		text := ""
		for _, seg := range event.Segs {
			text += seg.Text
		}
		if text != "" {
			lines = append(lines, TranscriptLine{
				Text:     strings.TrimSpace(text),
				Start:    float64(event.TStart) / 1000.0,
				Duration: float64(event.Dur) / 1000.0,
			})
		}
	}

	if len(lines) == 0 {
		return nil, fmt.Errorf("no transcript lines found")
	}

	return &Transcript{
		VideoID: videoID,
		Lines:   lines,
		Lang:    "en",
	}, nil
}

func parseVTTTranscript(videoID string, data []byte) (*Transcript, error) {
	lines := strings.Split(string(data), "\n")
	var transcript []TranscriptLine

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.Contains(line, "-->") {
			parts := strings.Split(line, "-->")
			if len(parts) == 2 {
				startStr := strings.TrimSpace(parts[0])
				start := vttTimeToSeconds(startStr)

				if i+1 < len(lines) {
					text := strings.TrimSpace(lines[i+1])
					if text != "" && !strings.Contains(text, "-->") {
						transcript = append(transcript, TranscriptLine{
							Text:     text,
							Start:    start,
							Duration: 0.5,
						})
					}
				}
			}
		}
	}

	if len(transcript) == 0 {
		return nil, fmt.Errorf("no transcript lines found in VTT")
	}

	return &Transcript{
		VideoID: videoID,
		Lines:   transcript,
		Lang:    "en",
	}, nil
}

func vttTimeToSeconds(timeStr string) float64 {
	timeStr = strings.ReplaceAll(timeStr, ",", ".")
	parts := strings.Split(timeStr, ":")
	if len(parts) != 3 {
		return 0
	}

	hours := parseFloat(parts[0])
	minutes := parseFloat(parts[1])
	seconds := parseFloat(parts[2])

	return hours*3600 + minutes*60 + seconds
}

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}
