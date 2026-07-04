package cli

import "fmt"

// This file centralizes the multi-line Usage/Description help text for the
// stage subcommands. Each helper renders the exact same string that was
// previously built inline in command.go, so command output is unchanged.

// statusDescription returns the Description text for the status command.
func statusDescription(cfg CommandConfig) string {
	return fmt.Sprintf(`Display staged changes for %s.

Without arguments, shows all staged %s changes.
With a %s name, shows the staged change for that specific %s.

Use --verbose to show detailed information including the staged value.

EXAMPLES:
   suve stage %s status              Show all staged %s changes
   suve stage %s status <name>       Show staged change for specific %s
   suve stage %s status --verbose    Show detailed information`,
		cfg.CommandName, cfg.ItemName, cfg.ItemName, cfg.ItemName,
		cfg.CommandName, cfg.ItemName,
		cfg.CommandName, cfg.ItemName,
		cfg.CommandName)
}

// diffDescription returns the Description text for the diff command.
func diffDescription(cfg CommandConfig) string {
	return fmt.Sprintf(`Compare staged values against AWS current values.

If a %s name is specified, shows diff for that %s only.
Otherwise, shows diff for all staged %ss.

EXAMPLES:
   suve stage %s diff                   Show diff for all staged %ss
   suve stage %s diff <name>            Show diff for specific %s
   suve stage %s diff --parse-json      Show diff with JSON formatting`,
		cfg.ItemName, cfg.ItemName, cfg.ItemName,
		cfg.CommandName, cfg.ItemName,
		cfg.CommandName, cfg.ItemName,
		cfg.CommandName)
}

// addDescription returns the Description text for the add command.
func addDescription(cfg CommandConfig) string {
	return fmt.Sprintf(`Create a new %s value and stage the change.

If value is provided as an argument, uses that value directly.
Otherwise, opens an editor to create the value.

If the %s is already staged for creation, edits the staged value.
The new %s will be created in AWS when you run 'suve stage %s apply'.

Use 'suve stage %s edit' to modify an existing %s.
Use 'suve stage %s status' to view staged changes.

EXAMPLES:
   suve stage %s add <name>              Open editor to create new %s
   suve stage %s add <name> <value>      Create new %s with given value`,
		cfg.ItemName,
		cfg.ItemName,
		cfg.ItemName, cfg.CommandName,
		cfg.CommandName, cfg.ItemName,
		cfg.CommandName,
		cfg.CommandName, cfg.ItemName,
		cfg.CommandName, cfg.ItemName)
}

// editDescription returns the Description text for the edit command.
func editDescription(cfg CommandConfig) string {
	return fmt.Sprintf(`Modify a %s value and stage the change.

If value is provided as an argument, uses that value directly.
Otherwise, opens an editor to modify the value.

If the %s is already staged, edits the staged value.
Otherwise, fetches the current value from AWS and opens it for editing.
Saves the edited value to the staging area (does not immediately apply to AWS).

Use 'suve stage %s delete' to stage a %s for deletion.
Use 'suve stage %s apply' to apply staged changes to AWS.
Use 'suve stage %s status' to view staged changes.

EXAMPLES:
   suve stage %s edit <name>              Open editor to modify %s
   suve stage %s edit <name> <value>      Set %s to given value`,
		cfg.ItemName,
		cfg.ItemName,
		cfg.CommandName, cfg.ItemName,
		cfg.CommandName,
		cfg.CommandName,
		cfg.CommandName, cfg.ItemName,
		cfg.CommandName, cfg.ItemName)
}

// applyDescription returns the Description text for the apply command.
func applyDescription(cfg CommandConfig) string {
	return fmt.Sprintf(`Apply all staged %s changes to AWS.

If a %s name is specified, only that %s's staged changes are applied.
Otherwise, all staged %s changes are applied.

After successful apply, the staged changes are cleared.

Use 'suve stage %s status' to view staged changes before applying.

CONFLICT DETECTION:
   Before applying, suve checks for conflicts to prevent lost updates:
   - For new resources: checks if someone else created it after staging
   - For existing resources: checks if it was modified after staging
   Use --ignore-conflicts to force apply despite conflicts.

EXAMPLES:
   suve stage %s apply                      Apply all staged %s changes (with confirmation)
   suve stage %s apply <name>               Apply only the specified %s
   suve stage %s apply --yes                Apply without confirmation
   suve stage %s apply --ignore-conflicts   Apply even if AWS was modified after staging`,
		cfg.ItemName,
		cfg.ItemName, cfg.ItemName,
		cfg.ItemName,
		cfg.CommandName,
		cfg.CommandName, cfg.ItemName,
		cfg.CommandName, cfg.ItemName,
		cfg.CommandName,
		cfg.CommandName)
}

// resetDescription returns the Description text for the reset command.
func resetDescription(cfg CommandConfig) string {
	return fmt.Sprintf(`Remove a %s from staging area or restore to a specific version.

Without a version specifier, the %s is simply removed from staging.
With a version specifier, the value at that version is fetched and staged.

Use 'suve stage %s reset --all' to unstage all %ss at once.

VERSION SPECIFIERS:
   <name>          Unstage %s (remove from staging)
   <name>#<ver>    Restore to specific version
   <name>~1        Restore to 1 version ago

EXAMPLES:
   suve stage %s reset <name>              Unstage (remove from staging)
   suve stage %s reset <name>#<ver>        Stage value from specific version
   suve stage %s reset <name>~1            Stage value from previous version
   suve stage %s reset --all               Unstage all %ss`,
		cfg.ItemName,
		cfg.ItemName,
		cfg.CommandName, cfg.ItemName,
		cfg.ItemName,
		cfg.CommandName,
		cfg.CommandName,
		cfg.CommandName,
		cfg.CommandName, cfg.ItemName)
}

// deleteDescription returns the Description text for the delete command.
// Secrets Manager (hasDeleteOptions) exposes recovery-window details that
// SSM Parameter Store does not.
func deleteDescription(cfg CommandConfig, hasDeleteOptions bool) string {
	if hasDeleteOptions {
		// Secrets Manager has delete options
		return fmt.Sprintf(`Stage a %s for deletion.

The %s will be deleted from AWS when you run 'suve stage %s apply'.
Use 'suve stage %s status' to view staged changes.
Use 'suve stage %s reset <name>' to unstage.

RECOVERY WINDOW:
   By default, %ss are scheduled for deletion after a 30-day recovery window.
   During this period, you can restore the %s using 'suve %s restore'.
   Use --force for immediate permanent deletion without recovery.

   Minimum: 7 days
   Maximum: 30 days
   Default: 30 days

EXAMPLES:
   suve stage %s delete <name>                      Stage with 30-day recovery
   suve stage %s delete --recovery-window 7 <name>  Stage with 7-day recovery
   suve stage %s delete --force <name>              Stage for immediate deletion`,
			cfg.ItemName,
			cfg.ItemName, cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.ItemName,
			cfg.ItemName, cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName)
	}

	// SSM Parameter Store doesn't have delete options
	return fmt.Sprintf(`Stage a %s for deletion.

The %s will be deleted from AWS when you run 'suve stage %s apply'.
Use 'suve stage %s status' to view staged changes.
Use 'suve stage %s reset <name>' to unstage.

EXAMPLES:
   suve stage %s delete <name>  Stage %s for deletion`,
		cfg.ItemName,
		cfg.ItemName, cfg.CommandName,
		cfg.CommandName,
		cfg.CommandName,
		cfg.CommandName, cfg.ItemName)
}

// tagDescription returns the Description text for the tag command.
func tagDescription(cfg CommandConfig) string {
	return fmt.Sprintf(`Stage tags to add or update for a %s.

Tags are staged locally and applied when you run 'suve stage %s apply'.
If the %s is not already staged, a tag-only change is created.

Use 'suve stage %s untag' to stage tag removals.
Use 'suve stage %s status' to view staged changes.

EXAMPLES:
   suve stage %s tag <name> env=prod              Stage single tag
   suve stage %s tag <name> env=prod team=api     Stage multiple tags`,
		cfg.ItemName,
		cfg.CommandName,
		cfg.ItemName,
		cfg.CommandName,
		cfg.CommandName,
		cfg.CommandName,
		cfg.CommandName)
}

// untagDescription returns the Description text for the untag command.
func untagDescription(cfg CommandConfig) string {
	return fmt.Sprintf(`Stage tags to remove from a %s.

Tag removals are staged locally and applied when you run 'suve stage %s apply'.
If the %s is not already staged, a tag-only change is created.

Use 'suve stage %s tag' to stage tag additions.
Use 'suve stage %s status' to view staged changes.

EXAMPLES:
   suve stage %s untag <name> env              Stage single tag removal
   suve stage %s untag <name> env team         Stage multiple tag removals`,
		cfg.ItemName,
		cfg.CommandName,
		cfg.ItemName,
		cfg.CommandName,
		cfg.CommandName,
		cfg.CommandName,
		cfg.CommandName)
}
