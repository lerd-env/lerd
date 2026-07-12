package reqstats

import "sync"

// The durable store is opened once per process and shared by every reader. The
// watcher writes it; lerd-ui and the TUI read it over the same WAL file, so a
// reader never blocks the writer.
var (
	sharedMu   sync.Mutex
	sharedPath string
	sharedRef  *Store
)

// OpenShared hands every caller in a process the same store handle. Only a
// successful open is cached, so a transient failure (the watcher hasn't created
// the file yet) is retried on the next call rather than memoised into a
// permanent outage.
func OpenShared(path string) (*Store, error) {
	sharedMu.Lock()
	defer sharedMu.Unlock()
	if sharedRef != nil && sharedPath == path {
		return sharedRef, nil
	}
	st, err := OpenStore(path)
	if err != nil {
		return nil, err
	}
	if sharedRef != nil {
		sharedRef.Close()
	}
	sharedPath, sharedRef = path, st
	return st, nil
}
