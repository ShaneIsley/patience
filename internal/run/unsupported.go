//go:build !unix

package run

// patience needs process groups and POSIX signals; it supports Linux and macOS only.
var _ = PatienceSupportsLinuxAndMacOSOnly
