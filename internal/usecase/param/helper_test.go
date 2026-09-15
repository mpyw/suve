package param_test

import "errors"

// Test sentinel errors for consistent error testing.
//
//declscope:package // a fixture the use-case test files share
var (
	errAWS              = errors.New("aws error")
	errGetParameter     = errors.New("get parameter error")
	errPutFailed        = errors.New("put failed")
	errDeleteFailed     = errors.New("delete failed")
	errHistoryFailed    = errors.New("history error")
	errAddTagsFailed    = errors.New("add tags failed")
	errRemoveTagsFailed = errors.New("remove tags failed")
	errAccessDenied     = errors.New("access denied")
	errUnexpectedCall   = errors.New("unexpected GetParameter call")
)
