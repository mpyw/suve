// Package azure provides use cases for the two Azure adapters (Key Vault secrets
// and App Configuration params).
//
// The use cases are written against the provider-neutral Reader/Writer/Store
// interfaces. Unlike the Google Cloud use cases they take a pre-reconstructed version
// suffix string (e.g. "#abc123", "~2", or "") rather than a typed spec: the two
// Azure services use different version grammars (Key Vault has opaque version
// ids; App Configuration has none), so decoupling the use cases from the spec
// type lets a single package serve both CLI groups. The CLI presenters own the
// typed spec and hand the suffix here.
// azure.go はパッケージ名を冠した共有部分（共通エラー）で、
// azure.ErrEntryNotFound -> AzureErrEntryNotFound のような stutter を避ける
// ため core とする。
//
//declscope:core
package azure

import "errors"

// ErrEntryNotFound is returned by the update use case when the target entry does
// not exist.
var ErrEntryNotFound = errors.New("entry not found")
