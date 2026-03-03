package ui

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"yt-tui/config"
	"yt-tui/storage"
	"yt-tui/youtube"
)

type App struct {
	app           *tview.Application
	cfg           *config.Config
	store         *storage.Storage
	pages         *tview.Pages
	searchBar     *tview.InputField
	videoList     *tview.List
	details       *tview.TextView
	statusBar     *tview.TextView
	playbackBar   *tview.TextView
	currentVideos []youtube.Video
	currentView   string
	mode          string
	focused       string
	loading       bool
	loadingText   string
	nowPlaying    string
	black         tcell.Color
	yellow        tcell.Color
	gray          tcell.Color
	cyan          tcell.Color
	green         tcell.Color
	white         tcell.Color
}

func NewApp(cfg *config.Config, store *storage.Storage) *App {
	app := tview.NewApplication()
	pages := tview.NewPages()

	black := tcell.GetColor("#000000")
	yellow := tcell.GetColor("#c18401")
	gray := tcell.GetColor("#696c77")
	cyan := tcell.GetColor("#199aa6")
	green := tcell.GetColor("#50a14f")
	white := tcell.GetColor("#696c77")

	tview.Styles.PrimitiveBackgroundColor = tcell.ColorDefault
	tview.Styles.ContrastBackgroundColor = tcell.ColorDefault
	tview.Styles.MoreContrastBackgroundColor = tcell.ColorDefault
	tview.Styles.PrimaryTextColor = black
	tview.Styles.SecondaryTextColor = gray
	tview.Styles.TertiaryTextColor = gray
	tview.Styles.InverseTextColor = black

	a := &App{
		app:         app,
		cfg:         cfg,
		store:       store,
		pages:       pages,
		currentView: "search",
		mode:        "normal",
		focused:     "list",
		black:       black,
		yellow:      yellow,
		gray:        gray,
		cyan:        cyan,
		green:       green,
		white:       white,
	}

	a.setupUI()
	return a
}

func (a *App) setupUI() {
	searchBar := tview.NewForm().
		AddInputField("Search: ", "", 40, nil, nil)
	searchBar.SetBorder(false)
	searchBar.SetBackgroundColor(tcell.ColorDefault)

	searchBar.GetFormItem(0).(*tview.InputField).SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			query := searchBar.GetFormItem(0).(*tview.InputField).GetText()
			if query != "" {
				a.currentView = "search"
				a.DoSearch(query)
				a.mode = "normal"
				a.app.SetFocus(a.videoList)
				a.updateStatusBar()
			}
		} else if key == tcell.KeyEscape {
			a.mode = "normal"
			a.app.SetFocus(a.videoList)
			a.updateStatusBar()
		}
	})
	a.searchBar = searchBar.GetFormItem(0).(*tview.InputField)
	a.searchBar.SetBackgroundColor(tcell.ColorDefault)
	a.searchBar.SetFieldBackgroundColor(tcell.ColorDefault)
	a.searchBar.SetLabelColor(a.cyan)

	a.videoList = tview.NewList().
		SetSelectedBackgroundColor(a.cyan).
		SetSelectedTextColor(a.black)
	a.videoList.SetBackgroundColor(tcell.ColorDefault)
	a.videoList.SetMainTextColor(a.black)
	a.videoList.SetSecondaryTextColor(a.gray)

	a.details = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetTextColor(a.black)
	a.details.SetBackgroundColor(tcell.ColorDefault)
	a.details.SetTextColor(a.black)

	a.statusBar = tview.NewTextView().
		SetDynamicColors(true).
		SetText("-- NORMAL -- j/k: down/up  h: back  l/Enter: play  i: search  P: playlists  q: quit")
	a.statusBar.SetBackgroundColor(tcell.ColorDefault)
	a.statusBar.SetTextColor(a.black)

	a.playbackBar = tview.NewTextView().
		SetDynamicColors(true).
		SetText("")
	a.playbackBar.SetBackgroundColor(tcell.ColorDefault)
	a.playbackBar.SetTextColor(a.black)

	searchPanel := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.searchBar, 1, 0, true).
		AddItem(a.videoList, 0, 3, false).
		AddItem(a.details, 0, 1, false)
	searchPanel.SetBorder(true).SetTitle("YouTube TUI")
	searchPanel.SetBackgroundColor(tcell.ColorDefault)

	mainFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(searchPanel, 0, 4, true).
		AddItem(a.playbackBar, 1, 0, false).
		AddItem(a.statusBar, 1, 0, false)
	mainFlex.SetBackgroundColor(tcell.ColorDefault)

	a.pages.AddPage("main", mainFlex, true, true)

	a.app.SetRoot(a.pages, true)
	a.app.SetInputCapture(a.handleInput)
}

func (a *App) handleInput(event *tcell.EventKey) *tcell.EventKey {
	if event.Key() == tcell.KeyCtrlC || (event.Key() == tcell.KeyRune && event.Rune() == 'q') {
		a.app.Stop()
		return nil
	}

	if event.Key() == tcell.KeyEscape {
		if a.pages.HasPage("playlists") {
			a.pages.HidePage("playlists")
			return nil
		}
		if a.pages.HasPage("addtoplaylist") {
			a.pages.HidePage("addtoplaylist")
			return nil
		}
		if a.pages.HasPage("create") {
			a.pages.HidePage("create")
			return nil
		}
		a.mode = "normal"
		a.app.SetFocus(a.videoList)
		a.updateStatusBar()
		return nil
	}

	if a.mode == "insert" {
		if event.Key() == tcell.KeyEscape {
			a.mode = "normal"
			a.app.SetFocus(a.videoList)
			a.updateStatusBar()
			return nil
		}
		return event
	}

	if event.Key() == tcell.KeyRune {
		r := event.Rune()
		if r == 'i' || r == '/' {
			a.mode = "insert"
			a.app.SetFocus(a.searchBar)
			a.updateStatusBar()
			return nil
		}

		if r == 'j' {
			a.videoList.SetCurrentItem(a.videoList.GetCurrentItem() + 1)
			if a.videoList.GetCurrentItem() < len(a.currentVideos) {
				a.showVideoDetails(a.currentVideos[a.videoList.GetCurrentItem()])
			}
			return nil
		}
		if r == 'k' {
			a.videoList.SetCurrentItem(a.videoList.GetCurrentItem() - 1)
			if a.videoList.GetCurrentItem() < len(a.currentVideos) {
				a.showVideoDetails(a.currentVideos[a.videoList.GetCurrentItem()])
			}
			return nil
		}
		if r == 'h' {
			if a.currentView == "playlist" {
				a.showPlaylists()
			} else {
				a.showPlaylists()
			}
			return nil
		}
		if r == 'l' || r == 'L' || r == ' ' {
			idx := a.videoList.GetCurrentItem()
			if idx >= 0 && idx < len(a.currentVideos) {
				a.playVideo(a.currentVideos[idx])
			}
			return nil
		}
		if r == 'p' || r == 'P' {
			a.showPlaylists()
			return nil
		}
		if r == 'r' || r == 'R' {
			if a.currentView == "history" {
				a.showHistory()
			}
			return nil
		}
		if r == 'G' {
			a.videoList.SetCurrentItem(len(a.currentVideos) - 1)
			if a.videoList.GetCurrentItem() < len(a.currentVideos) {
				a.showVideoDetails(a.currentVideos[a.videoList.GetCurrentItem()])
			}
			return nil
		}
		if r == 'g' {
			return event
		}
	}

	if event.Key() == tcell.KeyDown || event.Key() == tcell.KeyCtrlN {
		a.videoList.SetCurrentItem(a.videoList.GetCurrentItem() + 1)
		if a.videoList.GetCurrentItem() < len(a.currentVideos) {
			a.showVideoDetails(a.currentVideos[a.videoList.GetCurrentItem()])
		}
		return nil
	}
	if event.Key() == tcell.KeyUp || event.Key() == tcell.KeyCtrlP {
		a.videoList.SetCurrentItem(a.videoList.GetCurrentItem() - 1)
		if a.videoList.GetCurrentItem() < len(a.currentVideos) {
			a.showVideoDetails(a.currentVideos[a.videoList.GetCurrentItem()])
		}
		return nil
	}
	if event.Key() == tcell.KeyEnter {
		idx := a.videoList.GetCurrentItem()
		if idx >= 0 && idx < len(a.currentVideos) {
			a.playVideo(a.currentVideos[idx])
		}
		return nil
	}

	return event
}

func (a *App) updateStatusBar() {
	var base string
	if a.mode == "insert" {
		base = "-- INSERT -- Type search, Enter to search, Esc to exit"
	} else {
		base = "-- NORMAL -- j/k: down/up  h: playlists  l/Space/Enter: play  i: search  P: history  g: top  G: bottom  q: quit"
	}
	if a.loading {
		a.statusBar.SetText(base + " » " + a.loadingText)
	} else {
		a.statusBar.SetText(base)
	}
}

func (a *App) setLoading(loading bool, text string) {
	a.loading = loading
	a.loadingText = text
	a.updateStatusBar()
}

func (a *App) DoSearch(query string) {
	a.setLoading(true, "Searching "+query+"...")

	go func() {
		videos, err := youtube.Search(query, a.cfg.MaxResults)
		a.app.QueueUpdate(func() {
			defer a.setLoading(false, "")
			if err != nil {
				a.statusBar.SetText(fmt.Sprintf("Error: %s", err.Error()))
				return
			}

			a.currentVideos = videos
			a.videoList.Clear()

			for _, v := range videos {
				title := v.Title
				if len(title) > 50 {
					title = title[:50] + "..."
				}
				desc := fmt.Sprintf("[%s] %s", v.Duration, v.Channel)
				a.videoList.AddItem(title, desc, 0, nil)
			}

			a.updateStatusBar()

			if len(videos) > 0 {
				a.showVideoDetails(videos[0])
			}

			a.videoList.SetSelectedFunc(func(index int, mainText, secondaryText string, shortcut rune) {
				if index < len(a.currentVideos) {
					a.showVideoDetails(a.currentVideos[index])
				}
			})
		})
	}()
}

func (a *App) showVideoDetails(video youtube.Video) {
	details := fmt.Sprintf(`[cyan]Title:[] %s
[cyan]Channel:[] %s
[cyan]Duration:[] %s
[cyan]Views:[] %s
[cyan]Uploaded:[] %s

[cyan]Description:[]
%s`,
		video.Title,
		video.Channel,
		video.Duration,
		video.Views,
		video.Uploaded,
		video.Description,
	)
	a.details.SetText(details)
}

func (a *App) playVideo(video youtube.Video) {
	a.setLoading(true, "Getting stream for "+video.Title+"...")
	a.nowPlaying = video.Title

	go func() {
		url, err := youtube.GetStreamURL(video.URL)
		if err != nil {
			a.app.QueueUpdate(func() {
				defer a.setLoading(false, "")
				a.playbackBar.SetText(fmt.Sprintf("Error: %s", err.Error()))
			})
			return
		}

		a.app.QueueUpdate(func() {
			defer a.setLoading(false, "")
			a.playbackBar.SetTextColor(a.black)
			a.playbackBar.SetText(fmt.Sprintf("▶ Playing: %s", video.Title))
		})

		url = strings.TrimSpace(url)

		cmd := exec.Command(a.cfg.Player, "--no-video", "--quiet", url)
		cmd.Stdout = nil
		cmd.Stderr = nil
		cmd.Start()
		cmd.Wait()

		a.store.AddToHistory(video)

		a.app.QueueUpdate(func() {
			a.playbackBar.SetText("")
			a.nowPlaying = ""
			a.updateStatusBar()
		})
	}()
}

func (a *App) showPlaylists() {
	playlists, err := a.store.ListPlaylists()
	if err != nil {
		playlists = []string{}
	}

	buttons := []string{"Back", "View History"}
	for _, p := range playlists {
		buttons = append(buttons, p)
	}

	modal := tview.NewModal().
		SetText("Playlists\n\nSelect a playlist:").
		AddButtons(buttons).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			a.pages.HidePage("playlists")
			if buttonLabel == "View History" {
				a.showHistory()
			} else if buttonLabel != "Back" && buttonLabel != "" {
				a.showPlaylistVideos(buttonLabel)
			}
		})

	a.pages.AddPage("playlists", modal, false, true)
	a.pages.ShowPage("playlists")
}

func (a *App) showPlaylistVideos(name string) {
	videos, err := a.store.GetPlaylist(name)
	if err != nil || len(videos) == 0 {
		a.statusBar.SetText("Empty playlist")
		return
	}

	a.currentView = "playlist"
	a.currentVideos = videos
	a.videoList.Clear()

	for _, v := range videos {
		title := v.Title
		if len(title) > 50 {
			title = title[:50] + "..."
		}
		desc := fmt.Sprintf("[%s] %s", v.Duration, v.Channel)
		a.videoList.AddItem(title, desc, 0, nil)
	}

	a.statusBar.SetText("Enter[] play  Esc[] back")
}

func (a *App) showAddToPlaylistDialog(video youtube.Video) {
	playlists, _ := a.store.ListPlaylists()

	if len(playlists) == 0 {
		buttons := []string{"Cancel", "Create New"}
		modal := tview.NewModal().
			SetText(fmt.Sprintf("Add to playlist: %s\n\nNo playlists yet", video.Title)).
			AddButtons(buttons).
			SetDoneFunc(func(buttonIndex int, buttonLabel string) {
				a.pages.HidePage("addtoplaylist")
				if buttonLabel == "Create New" {
					a.showCreatePlaylistDialog(video)
				}
			})
		a.pages.AddPage("addtoplaylist", modal, false, true)
		a.pages.ShowPage("addtoplaylist")
		return
	}

	buttons := []string{"Cancel"}
	for _, p := range playlists {
		buttons = append(buttons, p)
	}
	buttons = append(buttons, "Create New")

	modal := tview.NewModal().
		SetText(fmt.Sprintf("Add to playlist: %s", video.Title)).
		AddButtons(buttons).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			a.pages.HidePage("addtoplaylist")
			if buttonLabel == "Create New" {
				a.showCreatePlaylistDialog(video)
			} else if buttonLabel != "Cancel" && buttonLabel != "" {
				a.store.AddToPlaylist(buttonLabel, video)
				a.statusBar.SetText(fmt.Sprintf("Added to %s", buttonLabel))
			}
		})

	a.pages.AddPage("addtoplaylist", modal, false, true)
	a.pages.ShowPage("addtoplaylist")
}

func (a *App) showCreatePlaylistDialog(video youtube.Video) {
	form := tview.NewForm().
		AddInputField("Playlist name", "", 30, nil, nil)
	form.SetBorder(true).SetTitle("Create Playlist")

	inputField := form.GetFormItem(0).(*tview.InputField)

	modal := tview.NewModal().
		SetText("Enter playlist name:").
		AddButtons([]string{"Cancel", "Create"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			a.pages.HidePage("create")
			if buttonLabel == "Create" {
				name := inputField.GetText()
				if name != "" {
					a.store.CreatePlaylist(name)
					a.store.AddToPlaylist(name, video)
					a.statusBar.SetText(fmt.Sprintf("Created and added to %s", name))
				}
			}
		})

	flex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(form, 3, 0, true).
		AddItem(modal, 3, 0, false)

	a.pages.AddPage("create", flex, false, true)
	a.pages.ShowPage("create")
}

func (a *App) showHistory() {
	a.setLoading(true, "Loading history...")

	go func() {
		history, err := a.store.GetHistory()
		a.app.QueueUpdate(func() {
			defer a.setLoading(false, "")
			if err != nil || len(history) == 0 {
				a.statusBar.SetText("No history yet.")
				return
			}

			a.currentView = "history"
			a.videoList.Clear()
			a.currentVideos = []youtube.Video{}

			for _, h := range history {
				a.currentVideos = append(a.currentVideos, h.Video)
				title := h.Video.Title
				if len(title) > 50 {
					title = title[:50] + "..."
				}
				desc := fmt.Sprintf("[%s] %s", h.Video.Duration, h.Video.Channel)
				a.videoList.AddItem(title, desc, 0, nil)
			}

			a.statusBar.SetText("R[] refresh  Enter[] play")
			a.updateStatusBar()
		})
	}()
}

func (a *App) Run() error {
	return a.app.Run()
}
