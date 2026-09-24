package param

import "strconv"

// versionNumber converts an SSM Parameter Store version id ("3") to the integer
// the JSON outputs carry. A non-numeric or empty id yields 0.
//
//declscope:package // diff, log and show render the version as a JSON number with it
func versionNumber(id string) int64 {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return 0
	}

	return n
}
