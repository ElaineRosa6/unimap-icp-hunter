package backup

import (
	"strconv"
	"strings"
	"time"
)

// isManagedBackupName recognizes the legacy second-resolution name and the
// current nanosecond timestamp plus CreateTemp decimal uint32 suffix. Matching
// the entire format keeps overlapping prefixes and unrelated archives separate.
func isManagedBackupName(name, prefix string) bool {
	rest, ok := strings.CutPrefix(name, prefix+"_backup_")
	if !ok {
		return false
	}
	stem, ok := strings.CutSuffix(rest, ".tar.gz")
	if !ok || len(stem) < 15 {
		return false
	}
	stamp, err := time.Parse("20060102_150405", stem[:15])
	if err != nil || stamp.Format("20060102_150405") != stem[:15] {
		return false
	}
	if len(stem) == 15 {
		return true
	}
	if len(stem) < 27 || len(stem) > 36 || stem[15] != '.' || stem[25] != '_' {
		return false
	}
	for _, digit := range stem[16:25] {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	suffix := stem[26:]
	for _, digit := range suffix {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	value, err := strconv.ParseUint(suffix, 10, 32)
	return err == nil && strconv.FormatUint(value, 10) == suffix
}
