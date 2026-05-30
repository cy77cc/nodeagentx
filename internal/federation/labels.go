package federation

import (
	"os"
	"runtime"
)

// CollectAutoLabels returns automatically detected labels for the current host.
func CollectAutoLabels() map[string]string {
	hostname, _ := os.Hostname()
	labels := map[string]string{
		"os":       runtime.GOOS,
		"arch":     runtime.GOARCH,
		"hostname": hostname,
	}
	if kernel := getKernelVersion(); kernel != "" {
		labels["kernel_version"] = kernel
	}
	return labels
}

// getKernelVersion reads the kernel version from /proc/version on Linux.
func getKernelVersion() string {
	if runtime.GOOS != "linux" {
		return ""
	}
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return ""
	}
	s := string(data)
	for i, c := range s {
		if c == '\n' {
			return s[:i]
		}
	}
	return s
}
