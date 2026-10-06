// White-box tests of helper.go.
//declscope:namespace helper

package param

import "errors"

// Test sentinel errors for consistent error testing.
//
//declscope:shared // a fixture the use-case test files share
var (
	errHelperAWS            = errors.New("aws error")
	errHelperGetParameter   = errors.New("get parameter error")
	errHelperPutFailed      = errors.New("put failed")
	errHelperDeleteFailed   = errors.New("delete failed")
	errHelperHistoryFailed  = errors.New("history error")
	errHelperAccessDenied   = errors.New("access denied")
	errHelperUnexpectedCall = errors.New("unexpected GetParameter call")
)
