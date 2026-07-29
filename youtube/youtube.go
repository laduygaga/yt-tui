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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	delim := "＜｜＞"
	recDelim := "＜ＲＥＣ＞"
	args := []string{
		"--no-check-certificate",
		"--flat-playlist",
		"--user-agent", userAgent,
		"--print", fmt.Sprintf("%%(id)s%s%%(title)s%s%%(channel)s%s%%(duration)s%s%%(view_count)s%s%%(upload_date)s%s%%(thumbnail)s%s%%(description)s%s%%(url)s%s",
			delim, delim, delim, delim, delim, delim, delim, delim, recDelim),
		"--", fmt.Sprintf("ytsearch%d:%s", maxResults, query),
	}

	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	output, err := cmd.Output()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("yt-dlp error: %s", string(exitError.Stderr))
		}
		return nil, fmt.Errorf("yt-dlp error: %w", err)
	}

	var videos []Video
	records := strings.Split(string(output), recDelim)
	for _, rec := range records {
		rec = strings.TrimSpace(rec)
		if rec == "" {
			continue
		}
		parts := strings.Split(rec, delim)
		if len(parts) >= 9 {
			id := parts[0]
			title := parts[1]
			duration := parts[3]
			url := parts[8]

			if id == "" || id == "NA" || len(id) != 11 {
				continue
			}
			if duration == "" || duration == "NA" || duration == "0:00" {
				if duration == "NA" {
					continue
				}
			}
			if !strings.Contains(url, "watch?v=") {
				continue
			}

			videos = append(videos, Video{
				ID:          id,
				Title:       title,
				Channel:     parts[2],
				Duration:    duration,
				Views:       parts[4],
				Uploaded:    parts[5],
				Thumbnail:   parts[6],
				Description: parts[7],
				URL:         url,
			})
		}
	}

	return videos, nil
}

func GetStreamURL(videoURL string, chromeProfile string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	tryExtract := func(profile, client string) (string, error) {
		args := []string{
			"--no-check-certificate",
			"--no-warnings",
			"--no-playlist",
			"-f", "bestaudio/best",
			"--print", "%(url)s",
		}
		if client != "" {
			args = append(args, "--extractor-args", fmt.Sprintf("youtube:player_client=%s", client))
		}
		if profile != "" {
			args = append(args, "--cookies-from-browser", fmt.Sprintf("chrome:%s", profile))
		}
		args = append(args, "--", videoURL)

		attemptCtx, attemptCancel := context.WithTimeout(ctx, 6*time.Second)
		defer attemptCancel()

		cmd := exec.CommandContext(attemptCtx, "yt-dlp", args...)
		output, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("yt-dlp error: %w", err)
		}
		url := strings.TrimSpace(string(output))
		if url == "" {
			return "", fmt.Errorf("empty stream URL")
		}
		return url, nil
	}

	strategies := []struct {
		profile string
		client  string
	}{
		{"", ""},
		{"", "android_vr"},
		{"", "tv_downgraded"},
		{"", "mediaconnect"},
		{"", "tv"},
		{chromeProfile, ""},
		{chromeProfile, "tv_downgraded"},
		{chromeProfile, "android_vr"},
		{chromeProfile, "mediaconnect"},
		{chromeProfile, "tv"},
	}

	for _, s := range strategies {
		if url, err := tryExtract(s.profile, s.client); err == nil && url != "" {
			return url, nil
		}
		if ctx.Err() != nil {
			return "", fmt.Errorf("yt-dlp timeout")
		}
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
