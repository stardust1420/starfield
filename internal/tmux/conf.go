package tmux

import "strings"

// RenderConf returns the private tmux config: Ghostty-friendly terminal
// features, no prefix key, a root-table detach key, and a themed status bar.
func RenderConf(detachKey string) string {
	var b strings.Builder
	w := func(s string) { b.WriteString(s + "\n") }
	w("# starfield private tmux config (generated; edits are overwritten)")
	w(`set -g default-terminal "tmux-256color"`)
	w(`set -as terminal-features ",xterm-ghostty:RGB:extkeys,xterm-256color:RGB"`)
	w("# extended keys so shift+enter reaches Claude")
	w("set -s extended-keys on")
	w(`set -as terminal-features 'xterm*:extkeys'`)
	w("set -s escape-time 0")
	w("set -g history-limit 100000")
	w("set -g mouse on")
	w("set -s set-clipboard on")
	w("set -g allow-passthrough on")
	w("set -g focus-events on")
	w("# no prefix; the only binding is the detach key")
	w("set -g prefix None")
	w("unbind C-b")
	w("bind -n " + detachKey + " detach-client")
	w("# status bar")
	w("set -g status on")
	w("set -g status-position bottom")
	w("set -g status-interval 5")
	w(`set -g status-style "bg=#1a1b26,fg=#a9b1d6"`)
	w(`set -g status-left-length 80`)
	w(`set -g status-left "#[fg=#d97757,bold] ✦ starfield #[fg=#565f89,nobold]│#[fg=#a9b1d6] #{b:@sf_dir} "`)
	w(`set -g status-right "#[fg=#565f89]#{?client_prefix,, ` + detachKey + ` ⏎ home }"`)
	w(`set -g window-status-format ""`)
	w(`set -g window-status-current-format ""`)
	return b.String()
}
