package aws

import (
	"os"

	"github.com/samber/lo"
)

// activeProfile names the shared-config profile the SDK loads credentials
// from, for display only: AWS_PROFILE, else AWS_DEFAULT_PROFILE. It returns ""
// when neither is set, or when AWS_ACCESS_KEY_ID is set, because the SDK then
// uses the environment credentials instead of any profile's. It never guesses a
// profile from ~/.aws/config: several profiles can share one account, and the
// account and region already identify the target.
//
//declscope:package // identity.go names the caller's profile with it
func activeProfile() string {
	if os.Getenv("AWS_ACCESS_KEY_ID") != "" {
		return ""
	}

	return lo.CoalesceOrEmpty(os.Getenv("AWS_PROFILE"), os.Getenv("AWS_DEFAULT_PROFILE"))
}
