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
	syncInterval         = 100 * time.Millisecond
	syncInitDelay        = 500 * time.Millisecond
	syncRetryInterval    = 500 * time.Millisecond
	durationSyncInterval = time.Second
	ipcTimeout           = 2 * time.Second
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

type ipcResponse struct {
	Data      json.RawMessage `json:"data"`
	Error     string          `json:"error"`
	RequestID int64           `json:"request_id"`
}

type Player struct {
	socketPath    string
	mu            sync.Mutex
	state         State
	snapshot      State
	snapshotVer   uint64
	cmd           *exec.Cmd
	conn          net.Conn
	decoder       *json.Decoder
	nextRequestID int64
	cancel        chan struct{}
	onEnded       func()
	lastError     string
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

	p.closeCancelLocked()
	p.cancel = make(chan struct{})
	cancel := p.cancel
	p.closeConnLocked()

	if p.cmd != nil {
		if p.cmd.Process != nil {
			p.cmd.Process.Kill()
		}
		p.cmd = nil
	}

	p.state = State{
		IsPlaying:     true,
		IsPaused:      false,
		IsLooping:     false,
		PlaybackSpeed: 1.0,
	}
	p.updateSnapshotLocked()
	p.onEnded = onEnded

	if _, err := os.Stat(p.socketPath); err == nil {
		os.Remove(p.socketPath)
	}

	p.cmd = exec.Command(playerCmd, "--no-video", "--no-input-terminal", "--no-terminal", "--quiet",
		fmt.Sprintf("--input-ipc-server=%s", p.socketPath), url)

	if err := p.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start player: %w", err)
	}

	cmd := p.cmd
	onEndedCb := p.onEnded
	go func() {
		cmd.Wait()

		p.mu.Lock()
		isCurrent := p.cmd == cmd
		if isCurrent {
			p.closeConnLocked()
			os.Remove(p.socketPath)
		}
		p.mu.Unlock()

		if isCurrent && onEndedCb != nil {
			onEndedCb()
		}
	}()

	go p.syncLoop(cancel)

	return nil
}

func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.closeCancelLocked()
	p.cancel = make(chan struct{})

	p.closeConnLocked()

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
	p.updateSnapshotLocked()
}

func (p *Player) TogglePause() error {
	p.mu.Lock()
	p.state.IsPaused = !p.state.IsPaused
	paused := p.state.IsPaused
	p.updateSnapshotLocked()
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
	p.updateSnapshotLocked()
	p.mu.Unlock()

	return p.sendCommand(map[string]interface{}{
		"command": []interface{}{"set_property", "speed", speed},
	})
}

func (p *Player) SetLoop(loop bool) error {
	p.mu.Lock()
	p.state.IsLooping = loop
	p.updateSnapshotLocked()
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
	p.updateSnapshotLocked()
	p.mu.Unlock()

	return p.sendCommand(map[string]interface{}{
		"command": []interface{}{"set_property", "pause", paused},
	})
}

func (p *Player) Snapshot() State {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.snapshot
}

func (p *Player) SnapshotVersion() uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.snapshotVer
}

func (p *Player) updateSnapshotLocked() {
	p.snapshot = p.state
	p.snapshotVer++
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

func (p *Player) closeCancelLocked() {
	if p.cancel == nil {
		return
	}

	select {
	case <-p.cancel:
	default:
		close(p.cancel)
	}
}

func (p *Player) closeConnLocked() {
	if p.conn != nil {
		p.conn.Close()
		p.conn = nil
	}
	p.decoder = nil
}

func (p *Player) syncLoop(cancel <-chan struct{}) {
	timer := time.NewTimer(syncInitDelay)
	defer timer.Stop()

	var lastDurationSync time.Time

	for {
		select {
		case <-cancel:
			return
		case <-timer.C:
		}

		p.mu.Lock()
		isPlaying := p.state.IsPlaying
		totalKnown := p.state.TotalTime > 0
		p.mu.Unlock()

		nextInterval := syncInterval

		if !isPlaying {
			timer.Reset(nextInterval)
			continue
		}

		fetchDuration := !totalKnown || time.Since(lastDurationSync) >= durationSyncInterval
		current, total, err := p.getTimePos(fetchDuration)
		if err != nil {
			nextInterval = syncRetryInterval
		} else if current >= 0 {
			if fetchDuration {
				lastDurationSync = time.Now()
			}

			p.mu.Lock()
			p.state.CurrentTime = current
			if total > 0 {
				p.state.TotalTime = total
			}
			p.updateSnapshotLocked()
			p.mu.Unlock()
		}

		timer.Reset(nextInterval)
	}
}

func (p *Player) getTimePos(fetchDuration bool) (float64, float64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	current, err := p.getPropertyLocked("time-pos")
	if err != nil {
		return -1, 0, err
	}

	if !fetchDuration {
		return current, 0, nil
	}

	total, _ := p.getPropertyLocked("duration")
	return current, total, nil
}

func (p *Player) getConn() (net.Conn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.getConnLocked()
}

func (p *Player) getConnLocked() (net.Conn, error) {
	if p.conn != nil {
		if p.decoder == nil {
			p.decoder = json.NewDecoder(p.conn)
		}
		return p.conn, nil
	}

	conn, err := net.DialTimeout("unix", p.socketPath, ipcTimeout)
	if err != nil {
		return nil, err
	}

	p.conn = conn
	p.decoder = json.NewDecoder(conn)
	return conn, nil
}

func (p *Player) sendCommand(cmd map[string]interface{}) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	_, err := p.ipcRequestLocked(cmd)
	return err
}

func (p *Player) getProperty(name string) (float64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.getPropertyLocked(name)
}

func (p *Player) getPropertyLocked(name string) (float64, error) {
	resp, err := p.ipcRequestLocked(map[string]interface{}{
		"command": []interface{}{"get_property", name},
	})
	if err != nil {
		return 0, err
	}

	if len(resp.Data) == 0 || string(resp.Data) == "null" {
		return 0, fmt.Errorf("mpv property %q unavailable", name)
	}

	var value float64
	if err := json.Unmarshal(resp.Data, &value); err != nil {
		return 0, err
	}

	return value, nil
}

func (p *Player) ipcRequestLocked(cmd map[string]interface{}) (ipcResponse, error) {
	conn, err := p.getConnLocked()
	if err != nil {
		return ipcResponse{}, err
	}

	p.nextRequestID++
	requestID := p.nextRequestID
	cmd["request_id"] = requestID

	data, err := json.Marshal(cmd)
	if err != nil {
		return ipcResponse{}, err
	}

	if err := conn.SetDeadline(time.Now().Add(ipcTimeout)); err != nil {
		p.closeConnLocked()
		return ipcResponse{}, err
	}

	if _, err := conn.Write(append(data, '\n')); err != nil {
		p.closeConnLocked()
		return ipcResponse{}, err
	}

	maxReads := 64
	for i := 0; i < maxReads; i++ {
		var resp ipcResponse
		if err := p.decoder.Decode(&resp); err != nil {
			p.closeConnLocked()
			return ipcResponse{}, err
		}

		if resp.RequestID != requestID {
			continue
		}

		if err := conn.SetDeadline(time.Time{}); err != nil {
			p.closeConnLocked()
			return ipcResponse{}, err
		}

		if resp.Error != "success" {
			return resp, fmt.Errorf("mpv error: %s", resp.Error)
		}

		return resp, nil
	}

	p.closeConnLocked()
	return ipcResponse{}, fmt.Errorf("ipc: too many unsolicited responses")
}
