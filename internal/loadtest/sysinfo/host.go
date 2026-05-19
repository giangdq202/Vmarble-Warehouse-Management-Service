package sysinfo

import "os"

// osHostname is split out so unit tests can substitute it without touching
// real syscalls. (No tests do that today, but the layer costs nothing.)
func osHostname() (string, error) {
	return os.Hostname()
}
