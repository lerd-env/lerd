package plugins

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// Permissions a plugin asks for in its manifest. What they allow runs only
// once the user says yes, and the list only grows, like events.
const (
	PermissionNotifications = "notifications"
	PermissionSchedule      = "schedule"
)

// Permissions lists them in the order every surface asks.
var Permissions = []string{PermissionNotifications, PermissionSchedule}

// The states of a permission, as every surface shows them.
const (
	PermissionNone    = "none"    // the plugin does not ask for it
	PermissionPending = "pending" // it asks, and the user has not answered
	PermissionAllowed = "allowed"
	PermissionDenied  = "denied"
)

// Asks reports whether the plugin's manifest asks for perm.
func (p *Plugin) Asks(perm string) bool {
	switch perm {
	case PermissionNotifications:
		return p.Notifications
	case PermissionSchedule:
		return len(p.Schedules) > 0
	}
	panic("plugins: unknown permission " + perm)
}

// PermissionState is where the plugin stands on perm. It is derived rather
// than stored, so an install or an update that starts asking is pending on its
// own, and an answer holds across updates until one changes what it covered.
func PermissionState(p *Plugin, perm string) string {
	if p == nil || !p.Asks(perm) {
		return PermissionNone
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		return PermissionPending
	}
	allowed, scope, asked := answer(cfg, p.Name, perm)
	switch {
	case !asked, scope != Scope(p, perm):
		return PermissionPending
	case allowed:
		return PermissionAllowed
	}
	return PermissionDenied
}

// SetPermission records the user's answer for an installed plugin that asks
// for perm, covering what it asks for now.
func SetPermission(name, perm string, allowed bool) error {
	p, err := Find(name)
	if err != nil {
		return err
	}
	return saveAnswer(p, perm, allowed, Scope(p, perm))
}

// AnswerAsShown records the answer to the question as the user saw it, shown
// being its Scope: refused if the plugin changed since, and saved as shown, so
// an update landing after the check is asked again rather than approved unseen.
func AnswerAsShown(name, perm string, allowed bool, shown string) error {
	p, err := Find(name)
	if err != nil {
		return err
	}
	if p.Asks(perm) && Scope(p, perm) != shown {
		return fmt.Errorf("%s changed what it asks for since you were asked; look again", name)
	}
	answerChecked()
	return saveAnswer(p, perm, allowed, shown)
}

// answerChecked is a seam for tests to land an update between AnswerAsShown's
// check and its save.
var answerChecked = func() {}

func saveAnswer(p *Plugin, perm string, allowed bool, scope string) error {
	if !p.Asks(perm) {
		return fmt.Errorf("plugin %s does not ask for %s", p.Name, permissionLabel[perm])
	}
	return config.UpdateGlobal(func(c *config.GlobalConfig) {
		// Notifications keep the key they were stored under before
		// permissions were general, so older binaries still read the answer.
		if perm == PermissionNotifications {
			c.SetPluginNotifications(p.Name, allowed)
			return
		}
		c.SetPluginPermission(p.Name, perm, allowed, scope)
	})
}

var permissionLabel = map[string]string{
	PermissionNotifications: "notifications",
	PermissionSchedule:      "running on a schedule",
}

func answer(cfg *config.GlobalConfig, name, perm string) (allowed bool, scope string, asked bool) {
	if perm == PermissionNotifications {
		allowed, asked = cfg.PluginNotifications(name)
		return allowed, "", asked
	}
	return cfg.PluginPermission(name, perm)
}

// Scope is what an answer to perm covers, so an update that changes it is
// asked again: for schedules, a hash of each one's name, interval and
// executable, a fixed size however many a plugin declares.
func Scope(p *Plugin, perm string) string {
	if perm != PermissionSchedule {
		return ""
	}
	parts := make([]string, 0, len(p.Schedules))
	for _, s := range p.Schedules {
		parts = append(parts, s.Name+" "+s.Every+" "+s.Run)
	}
	slices.Sort(parts)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:])
}

// forgetPermissions drops every answer about the plugin, so a reinstall asks
// again.
func forgetPermissions(name string) error {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	_, notif := cfg.PluginNotifications(name)
	if !notif && cfg.PluginPermissions[name] == nil {
		return nil
	}
	return config.UpdateGlobal(func(c *config.GlobalConfig) {
		c.ForgetPluginNotifications(name)
		c.ForgetPluginPermissions(name)
	})
}
