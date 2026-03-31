package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

func (m *Model) View() string {
	if m.showHelp {
		return m.helpView()
	}
	if m.showPlaylists {
		return m.playlistsView()
	}
	if m.showTranscriptView {
		return m.transcriptView()
	}

	var mainContent string
	var details string

	if m.mode == "insert" {
		mainContent = m.searchInput.View()
		if len(m.videos) > 0 {
			mainContent = lipgloss.JoinVertical(
				lipgloss.Left,
				mainContent,
				m.videoListView(),
			)
		}
	} else if m.loading {
		mainContent = statusStyle.Render("» " + m.loadingText)
	} else if len(m.videos) == 0 {
		mainContent = normalStyle.Render("No videos found")
	} else {
		mainContent = m.videoListView()
		details = m.detailsView()
	}

	layout := mainContent
	if details != "" {
		layout = lipgloss.JoinVertical(
			lipgloss.Left,
			mainContent,
			details,
		)
	}

	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(gray).
		Padding(0, 1)

	if m.width > 0 {
		border = border.Width(m.width - borderPadding)
	}

	content := border.Render(
		lipgloss.JoinVertical(
			lipgloss.Left,
			titleStyle.Render("YouTube TUI"+getTitleSuffix(m)),
			layout,
		),
	)

	var statusBar string
	isPlayerActive := m.player.IsPlaying()
	if isPlayerActive && m.nowPlaying != "" {
		status := "▶ Playing: "
		if m.player.IsPaused() {
			status = "⏸ Paused: "
		}
		if m.player.IsLooping() {
			status += "🔁 "
		}

		var progressStr string
		if m.totalTime > 0 {
			pct := m.currentTime / m.totalTime
			m.progress.Width = m.width - 20
			if m.progress.Width < 20 {
				m.progress.Width = 20
			}
			timeStr := fmt.Sprintf(" %s / %s", formatTime(m.currentTime), formatTime(m.totalTime))
			if m.playbackSpeed != defaultSpeed {
				timeStr += fmt.Sprintf(" [%.2fx]", m.playbackSpeed)
			}
			progressStr = "\n" + m.progress.ViewAs(pct) + timeStr
		} else if m.currentTime > 0 {
			timeStr := fmt.Sprintf(" %s / --:-- (Loading duration...)", formatTime(m.currentTime))
			if m.playbackSpeed != defaultSpeed {
				timeStr += fmt.Sprintf(" [%.2fx]", m.playbackSpeed)
			}
			progressStr = "\n" + timeStr
		}

		statusBar = lipgloss.NewStyle().Foreground(green).Render(status) + normalStyle.Render(m.nowPlaying) + progressStr
	} else if m.loading {
		statusBar = statusStyle.Render("» " + m.loadingText)
	} else {
		modeStr := "-- NORMAL --"
		if m.mode == "insert" {
			modeStr = "-- INSERT --"
		}
		statusBar = normalStyle.Render(modeStr + " j/k: navigate  h/l: seek  [/]: speed  p: pause  s: stop  ?: help  esc: quit")
	}

	if m.statusMsg != "" {
		statusBar = statusBar + "\n" + lipgloss.NewStyle().Foreground(yellow).Render("» "+m.statusMsg)
	}

	var subtitleSection string
	var hasSubtitleSection bool
	if m.nowPlaying != "" && m.showSubtitles {
		hasSubtitleSection = true
		if m.transcript == nil {
			subtitleSection = lipgloss.NewStyle().
				Foreground(lipgloss.Color("8")).
				Width(m.width - borderPadding).
				Align(lipgloss.Center).
				Render("[Loading subtitles...]")
		} else if len(m.transcript.Lines) == 0 {
			subtitleSection = lipgloss.NewStyle().
				Foreground(lipgloss.Color("8")).
				Width(m.width - borderPadding).
				Align(lipgloss.Center).
				Render("[No subtitles available]")
		} else {
			currentSub := m.getCurrentSubtitle()
			if currentSub != "" {
				subText := "▼ " + currentSub
				if len(subText) > m.width-10 {
					subText = subText[:m.width-13] + "..."
				}
				subtitleSection = lipgloss.NewStyle().
					Foreground(cyan).
					Width(m.width - borderPadding).
					Align(lipgloss.Center).
					Render(subText)
			} else {
				subtitleSection = " "
			}
		}
	}

	if hasSubtitleSection {
		return lipgloss.JoinVertical(
			lipgloss.Left,
			content,
			"",
			subtitleSection,
			"",
			statusBar,
		)
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		content,
		"",
		statusBar,
	)
}

func (m *Model) transcriptView() string {
	if m.transcript == nil || len(m.transcript.Lines) == 0 {
		return normalStyle.Render("No transcript available")
	}

	var lines []string
	lines = append(lines, titleStyle.Render("TRANSCRIPT (j/k to scroll, t to close)"))
	lines = append(lines, "")

	maxLines := m.height - 5
	if maxLines < 1 {
		maxLines = 1
	}

	endIdx := m.transcriptScrollIdx + maxLines
	if endIdx > len(m.transcript.Lines) {
		endIdx = len(m.transcript.Lines)
	}

	for i := m.transcriptScrollIdx; i < endIdx; i++ {
		line := m.transcript.Lines[i]
		timeStr := formatTime(line.Start)
		text := line.Text
		if len(text) > m.width-15 {
			text = text[:m.width-18] + "..."
		}

		isCurrent := m.currentTime >= (line.Start-subtitleStartOffset) && m.currentTime < (line.Start+line.Duration+subtitleEndOffset)
		if isCurrent {
			lines = append(lines, selectedStyle.Render(fmt.Sprintf("[%s] ▶ %s", timeStr, text)))
		} else {
			lines = append(lines, normalStyle.Render(fmt.Sprintf("[%s]   %s", timeStr, text)))
		}
	}

	if endIdx < len(m.transcript.Lines) {
		lines = append(lines, secondaryStyle.Render(fmt.Sprintf("... %d more lines (j to scroll)", len(m.transcript.Lines)-endIdx)))
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(cyan).
		Padding(1).
		Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

func (m *Model) helpView() string {
	help := `
  KEYBINDINGS
  -----------
  h/l:         Seek backward/forward 10s
  [/]:         Decrease/Increase playback speed (0.25x steps)
  L:           Toggle loop current song
  j/down:      Move down
  k/up:        Move up
  g:           Go to top
  G:           Go to bottom
  Enter/Spc:   Play video
  p:           Pause/Resume
  s:           Stop current song
  c:           Toggle captions/subtitles (while playing)
  t:           Toggle transcript view (while playing)
  y:           Copy URL to clipboard
  *:           Toggle Favorite
  dd:          Delete item
  Ctrl+L:      Clear list (History/Favorit)
  i/ /:        Search mode
  H:           Toggle History
  P:           Toggle Playlists menu
  ?:           Show/hide help
  q/esc:       Back / Normal mode / Quit
  Ctrl+C:      Force Quit
`
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(cyan).
		Padding(1).
		Render(help)
}

func (m *Model) videoListView() string {
	if len(m.videos) == 0 {
		return normalStyle.Render("No videos in current list")
	}

	var lines []string

	offset := uiOffsetBase
	if m.height < offset+2 {
		offset = m.height - 2
		if offset < 0 {
			offset = 0
		}
	}

	maxItems := (m.height - offset) / videoItemHeight
	if maxItems < 1 {
		maxItems = 1
	}

	endIdx := m.scrollIdx + maxItems
	if endIdx > len(m.videos) {
		endIdx = len(m.videos)
	}

	for i := m.scrollIdx; i < endIdx; i++ {
		v := m.videos[i]
		title := truncate(v.Title, m.width-10)
		desc := fmt.Sprintf("%s | %s", v.Duration, v.Channel)

		if i == m.selectedIdx {
			lines = append(lines, selectedStyle.Render("▶ "+title))
			lines = append(lines, selectedStyle.Render("  "+desc))
		} else {
			lines = append(lines, normalStyle.Render("  "+title))
			lines = append(lines, secondaryStyle.Render("  "+desc))
		}
	}

	if len(lines) == 0 {
		return normalStyle.Render("No videos to display")
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m *Model) detailsView() string {
	if len(m.videos) == 0 || m.selectedIdx >= len(m.videos) {
		return ""
	}
	v := m.videos[m.selectedIdx]

	width := m.width - 10
	if width < 10 {
		width = 10
	}

	return fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n\n%s\n%s",
		titleStyle.Render("Title:")+" "+normalStyle.Render(truncate(v.Title, width)),
		titleStyle.Render("Channel:")+" "+normalStyle.Render(truncate(v.Channel, width)),
		titleStyle.Render("Duration:")+" "+normalStyle.Render(v.Duration),
		titleStyle.Render("Views:")+" "+normalStyle.Render(v.Views),
		titleStyle.Render("Uploaded:")+" "+normalStyle.Render(v.Uploaded),
		titleStyle.Render("Description:"),
		secondaryStyle.Render(truncate(v.Description, width*3)),
	)
}

func (m *Model) getCurrentSubtitle() string {
	if m.transcript == nil || len(m.transcript.Lines) == 0 {
		return ""
	}

	for _, line := range m.transcript.Lines {
		startTime := line.Start - subtitleStartOffset
		endTime := line.Start + line.Duration + subtitleEndOffset
		if m.currentTime >= startTime && m.currentTime < endTime {
			return line.Text
		}
	}
	return ""
}

func (m *Model) playlistsView() string {
	var lines []string
	lines = append(lines, titleStyle.Render("Playlists"))
	lines = append(lines, "")
	lines = append(lines, secondaryStyle.Render("Select a playlist:"))
	lines = append(lines, "")

	offset := uiOffsetBase
	if m.height < offset+2 {
		offset = m.height - 2
		if offset < 0 {
			offset = 0
		}
	}

	maxHeight := m.height - offset
	if maxHeight < 1 {
		maxHeight = 1
	}

	for i, p := range m.playlists {
		if i < m.scrollIdx || i >= m.scrollIdx+maxHeight {
			continue
		}
		if i == m.selectedIdx {
			lines = append(lines, selectedStyle.Render("▶ "+p))
		} else {
			lines = append(lines, normalStyle.Render("  "+p))
		}
		videos, _ := m.store.GetPlaylist(p)
		lines = append(lines, secondaryStyle.Render(fmt.Sprintf("    %d videos", len(videos))))
	}

	lines = append(lines, "")
	lines = append(lines, secondaryStyle.Render("P/q/h/esc: back  j/k: navigate  Enter: select"))

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(gray).
		Padding(1)

	if m.width > 0 {
		style = style.Width(m.width - borderPadding)
	}

	return style.Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}
