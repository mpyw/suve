package provider

import (
	"strings"
)

// TargetSegment is one labeled part of a Target, such as "account" and its id.
// Value is empty when the part is unknown or unset.
type TargetSegment struct {
	Label string
	Value string
}

// Target describes, for display, what a scope points at: the ordered segments
// every UI shows as "who am I connected to" (the TUI status bar, the apply
// confirmation, the GUI sidebar, and the CLI confirmation prompts).
type Target struct {
	// Segments are in display order. Each provider always lists the same labels,
	// so a UI can keep a stable layout while some values are still empty.
	Segments []TargetSegment
	// Pending is true when some values need a network lookup that has not run
	// yet (AWS: the STS caller identity). Resolve it through
	// builtin.ResolveTarget.
	Pending bool
}

// String renders the segments that have a value as "label value", joined by
// " · ". It returns "" when no segment has a value.
// The package imports only the standard library, so this is a plain loop.
func (t Target) String() string {
	var parts []string

	for _, s := range t.Segments {
		if s.Value != "" {
			parts = append(parts, s.Label+" "+s.Value)
		}
	}

	return strings.Join(parts, " · ")
}

// Target describes the scope without any network call. An AWS scope that does
// not carry both account and region is Pending: those come from the STS caller
// identity, together with the profile the scope never carries.
func (s Scope) Target() Target {
	switch s.Provider {
	case ProviderAWS:
		t := AWSTarget("", s.AccountID, s.Region)
		t.Pending = s.AccountID == "" || s.Region == ""

		return t
	case ProviderGoogleCloud:
		return Target{Segments: []TargetSegment{{Label: "project", Value: s.ProjectID}}}
	case ProviderAzure:
		// The null namespace is the default, so the namespace is listed only
		// when one is selected.
		segments := []TargetSegment{{Label: "vault", Value: s.VaultName}, {Label: "store", Value: s.StoreName}}
		if s.AppConfigNamespace != "" {
			segments = append(segments, TargetSegment{Label: "namespace", Value: s.AppConfigNamespace})
		}

		return Target{Segments: segments}
	default:
		return Target{}
	}
}

// AWSTarget builds the AWS target from a resolved caller identity: the profile,
// account and region.
func AWSTarget(profile, accountID, region string) Target {
	return Target{Segments: []TargetSegment{
		{Label: "profile", Value: profile},
		{Label: "account", Value: accountID},
		{Label: "region", Value: region},
	}}
}
