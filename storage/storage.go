package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"yt-tui/config"
	"yt-tui/youtube"
)

type HistoryItem struct {
	Video     youtube.Video `json:"video"`
	WatchedAt time.Time     `json:"watched_at"`
}

type Storage struct {
	config      *config.Config
	historyFile string
	playlistDir string
}

func New(cfg *config.Config) *Storage {
	return &Storage{
		config:      cfg,
		historyFile: filepath.Join(cfg.DataDir, "history.json"),
		playlistDir: filepath.Join(cfg.DataDir, "playlists"),
	}
}

func (s *Storage) init() error {
	if err := os.MkdirAll(s.playlistDir, 0755); err != nil {
		return err
	}
	if _, err := os.Stat(s.historyFile); os.IsNotExist(err) {
		file, err := os.Create(s.historyFile)
		if err != nil {
			return err
		}
		file.WriteString("[]")
		file.Close()
	}
	return nil
}

func (s *Storage) AddToHistory(video youtube.Video) error {
	s.init()

	history, err := s.GetHistory()
	if err != nil {
		history = []HistoryItem{}
	}

	var newHistory []HistoryItem
	newHistory = append(newHistory, HistoryItem{Video: video, WatchedAt: time.Now()})

	for _, item := range history {
		isDuplicate := (item.Video.ID != "" && item.Video.ID == video.ID) ||
			(item.Video.URL != "" && item.Video.URL == video.URL)
		if isDuplicate {
			continue
		}
		newHistory = append(newHistory, item)
	}

	if len(newHistory) > 100 {
		newHistory = newHistory[:100]
	}

	data, err := json.MarshalIndent(newHistory, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.historyFile, data, 0644)
}

func (s *Storage) GetHistory() ([]HistoryItem, error) {
	s.init()

	data, err := os.ReadFile(s.historyFile)
	if err != nil {
		absPath, _ := filepath.Abs(s.historyFile)
		return nil, fmt.Errorf("failed to read history file at %s: %w", absPath, err)
	}

	var history []HistoryItem
	if err := json.Unmarshal(data, &history); err != nil {
		absPath, _ := filepath.Abs(s.historyFile)
		return nil, fmt.Errorf("failed to parse history JSON at %s: %w", absPath, err)
	}

	return history, nil
}

func (s *Storage) ClearHistory() error {
	s.init()
	return os.WriteFile(s.historyFile, []byte("[]"), 0644)
}

func (s *Storage) RemoveFromHistory(index int) error {
	s.init()

	history, err := s.GetHistory()
	if err != nil {
		return err
	}

	if index < 0 || index >= len(history) {
		return fmt.Errorf("invalid index")
	}

	history = append(history[:index], history[index+1:]...)

	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.historyFile, data, 0644)
}

func (s *Storage) CreatePlaylist(name string) error {
	s.init()
	path := filepath.Join(s.playlistDir, name+".json")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("playlist already exists")
	}
	return os.WriteFile(path, []byte("[]"), 0644)
}

func (s *Storage) AddToPlaylist(playlistName string, video youtube.Video) error {
	s.init()
	path := filepath.Join(s.playlistDir, playlistName+".json")

	var videos []youtube.Video
	data, err := os.ReadFile(path)
	if err == nil {
		json.Unmarshal(data, &videos)
	}

	videos = append(videos, video)

	data, err = json.MarshalIndent(videos, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

func (s *Storage) GetPlaylist(name string) ([]youtube.Video, error) {
	s.init()
	path := filepath.Join(s.playlistDir, name+".json")

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var videos []youtube.Video
	if err := json.Unmarshal(data, &videos); err != nil {
		return nil, err
	}

	return videos, nil
}

func (s *Storage) ListPlaylists() ([]string, error) {
	s.init()
	files, err := os.ReadDir(s.playlistDir)
	if err != nil {
		return nil, err
	}

	var playlists []string
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".json" {
			playlists = append(playlists, f.Name()[:len(f.Name())-5])
		}
	}

	return playlists, nil
}

func (s *Storage) RemoveFromPlaylist(playlistName string, index int) error {
	s.init()
	path := filepath.Join(s.playlistDir, playlistName+".json")

	videos, err := s.GetPlaylist(playlistName)
	if err != nil {
		return err
	}

	if index < 0 || index >= len(videos) {
		return fmt.Errorf("invalid index")
	}

	videos = append(videos[:index], videos[index+1:]...)

	data, err := json.MarshalIndent(videos, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

func (s *Storage) ToggleFavorite(video youtube.Video) (bool, error) {
	s.init()
	playlistName := "favorit"
	path := filepath.Join(s.playlistDir, playlistName+".json")

	videos, _ := s.GetPlaylist(playlistName)

	found := -1
	for i, v := range videos {
		if (v.ID != "" && v.ID == video.ID) || (v.URL != "" && v.URL == video.URL) {
			found = i
			break
		}
	}

	added := false
	if found != -1 {
		videos = append(videos[:found], videos[found+1:]...)
	} else {
		videos = append([]youtube.Video{video}, videos...)
		added = true
	}

	data, err := json.MarshalIndent(videos, "", "  ")
	if err != nil {
		return false, err
	}

	err = os.WriteFile(path, data, 0644)
	return added, err
}

func (s *Storage) ClearPlaylist(name string) error {
	s.init()
	path := filepath.Join(s.playlistDir, name+".json")
	return os.WriteFile(path, []byte("[]"), 0644)
}

func (s *Storage) DeletePlaylist(name string) error {
	s.init()
	path := filepath.Join(s.playlistDir, name+".json")
	return os.Remove(path)
}
