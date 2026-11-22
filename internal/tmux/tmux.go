// Package tmux drives starfield's private tmux server (-L starfield).
package tmux

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/stardust1420/starfield/internal/config"
)

const socketName = "starfield"

// Client runs tmux commands against the private socket.
type Client struct {
	Bin      string
	Socket   string
	ConfPath string

	// run executes tmux with args (everything after the binary). Tests
	// replace it; nil means run the real binary.
	run func(args ...string) ([]byte, error)
}

// Pane describes one managed tmux session.
type Pane struct {
	Session  string
	PanePID  int
	ClaudeID string
	Dir      string
	Created  time.Time
	Attached bool
}

// New writes the themed config to cfg.StateDir/tmux.conf and returns a Client.
func New(cfg config.Config) (*Client, error) {
	bin, err := exec.LookPath("tmux")
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.StateDir, 0o755); err != nil {
		return nil, err
	}
	conf := filepath.Join(cfg.StateDir, "tmux.conf")
	if err := os.WriteFile(conf, []byte(RenderConf(cfg.DetachKey)), 0o644); err != nil {
		return nil, err
	}
	return &Client{Bin: bin, Socket: socketName, ConfPath: conf}, nil
}

// SessionName is the tmux session name for a Claude session id.
func SessionName(claudeID string) string {
	if len(claudeID) > 8 {
		claudeID = claudeID[:8]
	}
	return "sf-" + claudeID
}

func (c *Client) base() []string {
	return []string{"-L", c.Socket, "-f", c.ConfPath}
}

func (c *Client) do(args ...string) ([]byte, error) {
	full := append(c.base(), args...)
	if c.run != nil {
		return c.run(full...)
	}
	return c.exec(full...)
}

func (c *Client) exec(args ...string) ([]byte, error) {
	cmd := exec.Command(c.Bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		// Keep stderr in the output so callers can classify the error.
		return stderr.Bytes(), err
	}
	return stdout.Bytes(), nil
}

func noServer(out []byte, err error) bool {
	s := string(out)
	if err != nil {
		s += err.Error()
	}
	return strings.Contains(s, "no server running") ||
		strings.Contains(s, "error connecting") ||
		strings.Contains(s, "No such file or directory")
}

const listFormat = "#{session_name}\t#{pane_pid}\t#{@sf_session}\t#{@sf_dir}\t#{session_created}\t#{session_attached}"

// List returns every pane on the private server; empty if none is running.
func (c *Client) List() ([]Pane, error) {
	out, err := c.do("list-panes", "-a", "-F", listFormat)
	if err != nil {
		if noServer(out, err) {
			return []Pane{}, nil
		}
		return nil, err
	}
	return parsePanes(string(out)), nil
}

func parsePanes(out string) []Pane {
	panes := []Pane{}
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 6 {
			continue
		}
		pid, _ := strconv.Atoi(f[1])
		created, _ := strconv.ParseInt(f[4], 10, 64)
		attached, _ := strconv.Atoi(f[5])
		panes = append(panes, Pane{
			Session:  f[0],
			PanePID:  pid,
			ClaudeID: f[2],
			Dir:      f[3],
			Created:  time.Unix(created, 0),
			Attached: attached > 0,
		})
	}
	return panes
}

// NewSession starts a detached session running argv (exec'd directly, no
// shell) in dir and tags it with the Claude id and dir.
func (c *Client) NewSession(name, dir, claudeID string, argv []string, width, height int) error {
	args := []string{"new-session", "-d", "-s", name, "-c", dir,
		"-x", strconv.Itoa(width), "-y", strconv.Itoa(height), "--"}
	args = append(args, argv...)
	if out, err := c.do(args...); err != nil {
		return fmt.Errorf("new-session: %w: %s", err, strings.TrimSpace(string(out)))
	}
	for _, kv := range [][2]string{{"@sf_session", claudeID}, {"@sf_dir", dir}} {
		if out, err := c.do("set-option", "-t", name, kv[0], kv[1]); err != nil {
			_, _ = c.do("kill-session", "-t", name)
			return fmt.Errorf("set-option %s: %w: %s", kv[0], err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

// AttachCmd returns the command that attaches to the session. TMUX is removed
// from the environment so it works when starfield itself runs inside tmux.
func (c *Client) AttachCmd(name string) *exec.Cmd {
	cmd := exec.Command(c.Bin, append(c.base(), "attach-session", "-t", name)...)
	env := make([]string, 0, len(os.Environ()))
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "TMUX=") {
			env = append(env, e)
		}
	}
	cmd.Env = env
	return cmd
}

// Kill ends the session.
func (c *Client) Kill(name string) error {
	if out, err := c.do("kill-session", "-t", name); err != nil {
		return fmt.Errorf("kill-session: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Capture returns the last lines of the session's pane, with escapes.
func (c *Client) Capture(name string, lines int) (string, error) {
	out, err := c.do("capture-pane", "-p", "-e", "-t", name, "-S", "-"+strconv.Itoa(lines))
	if err != nil {
		return "", err
	}
	return string(out), nil
}
