// Package disk reports filesystem usage for configured download directories
// (rtorrent has no native disk-free RPC).
package disk

import "golang.org/x/sys/unix"

type Info struct {
	Path  string `json:"path"`
	Total uint64 `json:"total"`
	Free  uint64 `json:"free"`
	Used  uint64 `json:"used"`
}

// Usage statfs's each directory; unreadable paths are skipped.
func Usage(dirs []string) []Info {
	out := make([]Info, 0, len(dirs))
	for _, d := range dirs {
		var st unix.Statfs_t
		if err := unix.Statfs(d, &st); err != nil {
			continue
		}
		bs := uint64(st.Bsize)
		total := st.Blocks * bs
		free := st.Bavail * bs
		out = append(out, Info{Path: d, Total: total, Free: free, Used: total - free})
	}
	return out
}
