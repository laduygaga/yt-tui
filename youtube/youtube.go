package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

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
	Transcript  *Transcript `json:"-"`
}

func Search(query string, maxResults int) ([]Video, error) {
	args := []string{
		"--no-check-certificate",
		"--flat-playlist",
		"--user-agent", userAgent,
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

func GetStreamURL(videoURL string, chromeProfile string) (string, error) {
	args := []string{
		"--no-check-certificate",
		"--no-warnings",
		"--no-playlist",
		"-f", "bestaudio/best",
		"--print", "%(url)s",
	}

	if chromeProfile != "" {
		args = append(args, "--cookies-from-browser", fmt.Sprintf("chrome:%s", chromeProfile))
	}

	args = append(args, "--", videoURL)

	cmd := exec.Command("yt-dlp", args...)
	output, err := cmd.Output()
	if err == nil {
		url := strings.TrimSpace(string(output))
		if url != "" {
			return url, nil
		}
	}

	configs := []struct {
		client string
		skip   string
	}{
		{"android_vr", "webpage,consent"},
		{"mediaconnect", "webpage,consent"},
		{"tv", "webpage,consent"},
	}

	for _, cfg := range configs {
		args := []string{
			"--no-check-certificate",
			"--no-warnings",
			"--no-playlist",
			"-f", "bestaudio/best",
			"--print", "%(url)s",
			"--extractor-args", fmt.Sprintf("youtube:player_client=%s;player_skip=%s", cfg.client, cfg.skip),
			"--", videoURL,
		}

		cmd := exec.Command("yt-dlp", args...)
		output, err := cmd.Output()
		if err == nil {
			url := strings.TrimSpace(string(output))
			if url != "" {
				return url, nil
			}
		}

		if exitError, ok := err.(*exec.ExitError); ok {
			if cfg.client == configs[len(configs)-1].client {
				return "", fmt.Errorf("yt-dlp error: %s", string(exitError.Stderr))
			}
			continue
		}

		return "", fmt.Errorf("yt-dlp error: %w", err)
	}

	return "", fmt.Errorf("yt-dlp error: all methods failed")
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
