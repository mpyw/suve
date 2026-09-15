// delete.go の非公開関数を外部テストへ公開する export シム。
//declscope:namespace delete

package delete

// ValidateDeleteFlags exposes the unexported validateDeleteFlags for testing.
//
//nolint:gochecknoglobals // test-only export hook
var ValidateDeleteFlags = validateDeleteFlags
