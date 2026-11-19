package podman

// WebServerUnit is the container and systemd unit of the web server that
// serves lerd's sites; everything that starts, probes, binds or logs it uses
// this name rather than spelling it out.
const WebServerUnit = "lerd-nginx"
