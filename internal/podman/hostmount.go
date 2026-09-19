package podman

// HostMountRunArgs returns the podman run flags a throwaway container needs
// before it can read a host path bind-mounted into it.
//
// On an SELinux distribution (Fedora, RHEL, CentOS, Rocky and the ostree images
// built from them) a container runs as container_t, which may read usr_t but not
// the data_home_t that everything under the user's home carries. The bind mount
// still succeeds, so the file is visibly present inside the container and every
// read of it is denied, and the error surfaces as whatever the tool makes of a
// permission failure rather than as anything about labelling.
//
// lerd's quadlets already opt out of labelling with the same flag, which is why
// the long-running containers never hit this. A one-off `podman run` assembles
// its own arguments and so has to say it too, wherever it mounts a host path.
func HostMountRunArgs() []string {
	return []string{"--security-opt", "label=disable"}
}
