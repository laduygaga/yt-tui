package player

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"sync"
	"time"
)

const (
	syncInterval  = 100 * time.Millisecond
	syncInitDelay = 500 * time.Millisecond
)

type State struct {
	IsPlaying     bool
	IsPaused      bool
	IsLooping     bool
	PlaybackSpeed float64
	CurrentTime   float64
	TotalTime     float64
}

type ProgressUpdate struct {
	Current float64
	Total   float64
}

type Player struct {
	socketPath string
	mu         sync.Mutex
	state      State
	cmd        *exec.Cmd
	conn       net.Conn
	cancel     chan struct{}
	onEnded    func()
}

func New(socketPath string) *Player {
	return &Player{
		socketPath: socketPath,
		cancel:     make(chan struct{}),
		state: State{
			PlaybackSpeed: 1.0,
		},
	}
}

func (p *Player) Start(url string, playerCmd string, onEnded func()) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Kill()
		p.cmd.Wait()
	}

	p.state = State{
		IsPlaying:     true,
		IsPaused:      false,
		IsLooping:     false,
		PlaybackSpeed: 1.0,
	}
	p.onEnded = onEnded

	os.Remove(p.socketPath)

	p.cmd = exec.Command(playerCmd, "--no-video", "--quiet", "--ytdl",
		fmt.Sprintf("--input-ipc-server=%s", p.socketPath), url)

	if err := p.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start player: %w", err)
	}

	go func() {
		p.cmd.Wait()
		os.Remove(p.socketPath)
		if p.onEnded != nil {
			p.onEnded()
		}
	}()

	go p.syncLoop()

	return nil
}

func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	close(p.cancel)
	p.cancel = make(chan struct{})

	if p.conn != nil {
		p.conn.Close()
		p.conn = nil
	}

	if p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Kill()
		p.cmd = nil
	}

	p.state = State{
		IsPlaying:     false,
		IsPaused:      false,
		IsLooping:     false,
		PlaybackSpeed: 1.0,
		CurrentTime:   0,
		TotalTime:     0,
	}
}

func (p *Player) TogglePause() error {
	p.mu.Lock()
	p.state.IsPaused = !p.state.IsPaused
	paused := p.state.IsPaused
	p.mu.Unlock()

	return p.sendCommand(map[string]interface{}{
		"command": []interface{}{"set_property", "pause", paused},
	})
}

func (p *Player) Seek(seconds float64) error {
	return p.sendCommand(map[string]interface{}{
		"command": []interface{}{"seek", seconds, "relative"},
	})
}

func (p *Player) SetSpeed(speed float64) error {
	p.mu.Lock()
	p.state.PlaybackSpeed = speed
	p.mu.Unlock()

	return p.sendCommand(map[string]interface{}{
		"command": []interface{}{"set_property", "speed", speed},
	})
}

func (p *Player) SetLoop(loop bool) error {
	p.mu.Lock()
	p.state.IsLooping = loop
	p.mu.Unlock()

	loopVal := "inf"
	if !loop {
		loopVal = "no"
	}

	return p.sendCommand(map[string]interface{}{
		"command": []interface{}{"set_property", "loop-file", loopVal},
	})
}

func (p *Player) SetPaused(paused bool) error {
	p.mu.Lock()
	p.state.IsPaused = paused
	p.mu.Unlock()

	return p.sendCommand(map[string]interface{}{
		"command": []interface{}{"set_property", "pause", paused},
	})
}

func (p *Player) GetState() State {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

func (p *Player) IsPlaying() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state.IsPlaying
}

func (p *Player) IsPaused() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state.IsPaused
}

func (p *Player) IsLooping() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state.IsLooping
}

func (p *Player) PlaybackSpeed() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state.PlaybackSpeed
}

func (p *Player) Close() {
	p.Stop()
}

func (p *Player) syncLoop() {
	time.Sleep(syncInitDelay)

	for {
		select {
		case <-p.cancel:
			return
		default:
		}

		p.mu.Lock()
		isPlaying := p.state.IsPlaying
		isPaused := p.state.IsPaused
		p.mu.Unlock()

		if !isPlaying || isPaused {
			time.Sleep(syncInterval)
			continue
		}

		current, total := p.getTimePos()
		if current >= 0 {
			p.mu.Lock()
			p.state.CurrentTime = current
			if total > 0 {
				p.state.TotalTime = total
			}
			p.mu.Unlock()
		}

		time.Sleep(syncInterval)
	}
}

func (p *Player) getTimePos() (float64, float64) {
	conn, err := p.getConn()
	if err != nil {
		return -1, 0
	}

	current, err := p.getProperty(conn, "time-pos")
	if err != nil {
		return -1, 0
	}

	total, _ := p.getProperty(conn, "duration")
	return current, total
}

func (p *Player) getConn() (net.Conn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.conn != nil {
		return p.conn, nil
	}

	conn, err := net.DialTimeout("unix", p.socketPath, 2*time.Second)
	if err != nil {
		return nil, err
	}

	p.conn = conn
	return conn, nil
}

func (p *Player) sendCommand(cmd map[string]interface{}) error {
	conn, err := p.getConn()
	if err != nil {
		return err
	}

	data, err := json.Marshal(cmd)
	if err != nil {
		return err
	}

	_, err = conn.Write(append(data, '\n'))
	return err
}

func (p *Player) getProperty(conn net.Conn, name string) (float64, error) {
	cmd := map[string]interface{}{
		"command": []interface{}{"get_property", name},
	}

	data, err := json.Marshal(cmd)
	if err != nil {
		return 0, err
	}

	_, err = conn.Write(append(data, '\n'))
	if err != nil {
		p.mu.Lock()
		p.conn = nil
		p.mu.Unlock()
		conn.Close()
		return 0, err
	}

	var resp struct {
		Data  float64 `json:"data"`
		Error string  `json:"error"`
	}

	decoder := json.NewDecoder(conn)
	if err := decoder.Decode(&resp); err != nil {
		return 0, err
	}

	if resp.Error != "success" {
		return 0, fmt.Errorf("mpv error: %s", resp.Error)
	}

	return resp.Data, nil
}
