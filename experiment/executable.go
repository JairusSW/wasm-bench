package experiment

import (
	"runtime"
	"strings"
)

// NativeExecutable names a runnable native program on the controller's OS.
// Archive object names remain platform-neutral data; only restored/staged
// executable filenames need the native suffix. This does not convert binaries.
func NativeExecutable(path string) string { return ExecutableForOS(path, runtime.GOOS) }

func ExecutableForOS(path, goos string) string {
	if goos == "windows" && !strings.HasSuffix(strings.ToLower(path), ".exe") {
		return path + ".exe"
	}
	return path
}
