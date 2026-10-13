// Package version includes the version information.
package version

import (
	"fmt"
	"runtime"

	"github.com/go-logr/logr"
)

var (
	// Raw is the string representation of the version. This will be replaced
	// with the calculated version at build time.
	// set in the Makefile.
	Raw = "was not built with version info"

	// String is the human-friendly representation of the version.
	String = fmt.Sprintf("openshift/image-customization-controller %s", Raw)

	// Commit is the commit hash from which the software was built.
	// Set via LDFLAGS in Makefile.
	Commit = "unknown"

	// BuildTime is the string representation of build time.
	// Set via LDFLAGS in Makefile.
	BuildTime = "unknown"
)

func Print(log logr.Logger) {
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
	log.Info(fmt.Sprintf("Git commit: %s", Commit))
	log.Info(fmt.Sprintf("Build time: %s", BuildTime))
	log.Info(fmt.Sprintf("Component: %s", String))
}
