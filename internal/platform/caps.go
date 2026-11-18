package platform

// Caps are the facts about the host OS that shared code branches on. Each OS
// sets its own in a caps_<os>.go file as Current.
type Caps struct {
	// Opener hands a URL, file or folder to the desktop's default app; empty
	// where there is none.
	Opener string
	// UsesMachineVM: containers run inside a VM, so host unix sockets and file
	// events don't reach them and gvproxy forwards their ports.
	UsesMachineVM bool
	// PrivilegedPortsBindAll: a host port below 1024 can only be published on
	// every interface (gvproxy runs as the user, and macOS lets only root bind
	// one address there), so a loopback bind on it reaches the LAN as well.
	PrivilegedPortsBindAll bool
	// NativePHPRuntime: PHP can run on the host instead of in a container.
	NativePHPRuntime bool
	// WorkerModes: workers can run through exec or in a container of their own.
	WorkerModes bool
	// TrayHost: the desktop can show lerd's own tray applet.
	TrayHost bool
	// WindowsSide: Linux inside WSL, with lerd.exe, its agent and the desktop app
	// on the Windows side to keep in step and to serve.
	WindowsSide bool
}
