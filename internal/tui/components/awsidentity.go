package components

// AWSIdentity carries the async-resolved AWS caller identity shown in the
// status bar. It is a plain data struct (no SDK types) so the status bar never
// depends on the AWS provider package; the launch layer fills it via an
// injected fetcher.
type AWSIdentity struct {
	Account string
	Region  string
	Profile string
}
