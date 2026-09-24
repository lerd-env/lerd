// Package screenshare tells whether the desktop is being shared right now.
// Only Wayland can answer: every share there goes through the compositor's
// screencast, which it publishes as a PipeWire video stream.
package screenshare

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
)

// Supported reports whether this host can detect a share at all.
func Supported() bool {
	_, err := exec.LookPath("pw-dump")
	return err == nil
}

// desktopShells consume screencasts for their own previews: Plasma draws its
// taskbar hover thumbnails from kwin-screencast streams. Those are not a share.
var desktopShells = map[string]bool{"plasmashell": true}

type object struct {
	ID   int    `json:"id"`
	Type string `json:"type"`
	Info *struct {
		Props        map[string]any `json:"props"`
		OutputNodeID int            `json:"output-node-id"`
		InputNodeID  int            `json:"input-node-id"`
	} `json:"info"`
}

// graph is the PipeWire objects pw-dump has reported, by id.
type graph map[int]object

func (g graph) apply(objs []object) {
	for _, o := range objs {
		if o.Info == nil {
			delete(g, o.ID)
			continue
		}
		g[o.ID] = o
	}
}

func (g graph) prop(id int, key string) any {
	o, ok := g[id]
	if !ok || o.Info == nil {
		return nil
	}
	return o.Info.Props[key]
}

// producesScreen: compositors publish a share as a video output stream (KWin,
// mutter, niri) or as a Video/Source with no device.api (xdpw); webcams carry
// v4l2 or libcamera there.
func (g graph) producesScreen(id int) bool {
	switch g.prop(id, "media.class") {
	case "Stream/Output/Video":
		return true
	case "Video/Source":
		return g.prop(id, "device.api") == nil
	}
	return false
}

// active: a screencast only leaves the machine once something outside the
// desktop shell is linked to it, the browser, OBS or meeting app doing the share.
func (g graph) active() bool {
	for _, o := range g {
		if o.Type != "PipeWire:Interface:Link" {
			continue
		}
		consumer, _ := g.prop(o.Info.InputNodeID, "node.name").(string)
		if g.producesScreen(o.Info.OutputNodeID) && !desktopShells[consumer] {
			return true
		}
	}
	return false
}

// ParseActive reads one pw-dump snapshot.
func ParseActive(dump []byte) (bool, error) {
	var objs []object
	if err := json.Unmarshal(dump, &objs); err != nil {
		return false, fmt.Errorf("reading pw-dump output: %w", err)
	}
	g := graph{}
	g.apply(objs)
	return g.active(), nil
}

// Follow runs `pw-dump -m` and calls onChange with the first state and then on
// every flip, as the graph changes rather than on a poll, so a share is seen
// the moment its stream is linked. It returns when pw-dump exits.
func Follow(onChange func(active bool)) error {
	cmd := exec.Command("pw-dump", "-m")
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("pw-dump -m: %w", err)
	}
	ferr := follow(out, onChange)
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	return ferr
}

func follow(r io.Reader, onChange func(active bool)) error {
	dec := json.NewDecoder(r)
	g := graph{}
	first, last := true, false
	for {
		var objs []object
		if err := dec.Decode(&objs); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("reading pw-dump output: %w", err)
		}
		g.apply(objs)
		if now := g.active(); first || now != last {
			first, last = false, now
			onChange(now)
		}
	}
}
