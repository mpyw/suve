package staging

import "errors"

// ErrValueNotUTF8 is returned when a value to be staged is not valid UTF-8.
// The staging state stores values as UTF-8 strings (mirroring jsonutil, which
// refuses to format invalid UTF-8 to avoid U+FFFD coercion), so binary values
// cannot be staged. This covers every ingestion path: positional argv, the
// $EDITOR fallback, and provider prefill.
var ErrValueNotUTF8 = errors.New("value is not valid UTF-8: binary values cannot be staged")
