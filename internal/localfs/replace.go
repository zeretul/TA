package localfs

import (
	"os"
	"time"
)

// ReplaceFile renames source over destination without removing the destination
// first. Windows readers and scanners can briefly deny replacement even after
// the writer has closed its handle, so only those transient errors are retried.
// Permanent failures leave the original destination and the error intact.
func ReplaceFile(source, destination string) error {
	for attempt := 0; ; attempt++ {
		err := os.Rename(source, destination)
		if err == nil || attempt == 6 || !retryableReplaceError(err) {
			return err
		}
		time.Sleep((10 * time.Millisecond) << attempt)
	}
}
