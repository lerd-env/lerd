package plugins

// The states of a plugin's notifications, kept by these names for the callers
// that predate general permissions.
const (
	NotificationsNone    = PermissionNone
	NotificationsPending = PermissionPending
	NotificationsAllowed = PermissionAllowed
	NotificationsDenied  = PermissionDenied
)

// NotificationState is where the plugin stands on sending notifications.
func NotificationState(p *Plugin) string { return PermissionState(p, PermissionNotifications) }

// SetNotifications records the user's answer for an installed plugin that asks
// to send notifications.
func SetNotifications(name string, allowed bool) error {
	return SetPermission(name, PermissionNotifications, allowed)
}
