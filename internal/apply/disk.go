package apply

import (
	"fmt"
	"math"
	"syscall"
)

const (
	minFreeMiB = 200
	mib        = 1024 * 1024
)

func freeBytes(path string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, fmt.Errorf("apply: statfs %s: %w", path, err)
	}
	signed := int64(st.Bsize)
	if signed <= 0 {
		return 0, fmt.Errorf("apply: statfs block size is %d bytes", signed)
	}
	bsize := uint64(signed)
	if st.Bavail > math.MaxUint64/bsize {
		return math.MaxUint64, nil
	}
	return st.Bavail * bsize, nil
}
