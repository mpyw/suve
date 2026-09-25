// help.go holds the stage subcommands' long help texts. command.go builds each
// subcommand from them, and export.go and import.go render theirs with
// renderHelp, so the whole file is package-wide.
//declscope:package

package cli

import "strings"

// This file centralizes the multi-line Usage/Description help text for the
// stage subcommands. Each text is a template rendered by renderHelp:
//
//	{path}     the service's stage command path (CommandConfig.CommandPath)
//	{service}  the service's immediate command path (helpServicePath)
//	{item}     the item noun (CommandConfig.ItemName)
//	{name}     the service command name (CommandConfig.CommandName)
//	{provider} the provider label (CommandConfig.ProviderLabel)

// renderHelp fills the help-text placeholders from the command config.
func renderHelp(cfg CommandConfig, text string) string {
	return strings.NewReplacer(
		"{path}", cfg.CommandPath,
		"{service}", helpServicePath(cfg),
		"{item}", cfg.ItemName,
		"{name}", cfg.CommandName,
		"{provider}", remoteName(cfg.ProviderLabel),
	).Replace(text)
}

// helpServicePath derives the immediate (non-staging) command path of the
// service from its stage path, e.g. "suve aws stage secret" -> "suve aws secret".
//
//declscope:private
func helpServicePath(cfg CommandConfig) string {
	return strings.Replace(cfg.CommandPath, " stage", "", 1)
}

// statusHelp returns the Description text for the status command.
func statusHelp(cfg CommandConfig) string {
	return renderHelp(cfg, `Display staged {item} changes.

Without arguments, shows all staged {item} changes.
With a {item} name, shows the staged change for that specific {item}.

Use --verbose to show detailed information including the staged value.

EXAMPLES:
   {path} status              Show all staged {item} changes
   {path} status <name>       Show staged change for specific {item}
   {path} status --verbose    Show detailed information`)
}

// diffHelp returns the Description text for the diff command.
func diffHelp(cfg CommandConfig) string {
	return renderHelp(cfg, `Compare staged values against current {provider} values.

If a {item} name is specified, shows diff for that {item} only.
Otherwise, shows diff for all staged {item}s.

EXAMPLES:
   {path} diff                   Show diff for all staged {item}s
   {path} diff <name>            Show diff for specific {item}
   {path} diff --parse-json      Show diff with JSON formatting`)
}

// addHelp returns the Description text for the add command.
func addHelp(cfg CommandConfig) string {
	return renderHelp(cfg, `Create a new {item} value and stage the change.

If value is provided as an argument (or piped in with --value-stdin), uses
that value directly, even when it is empty. Otherwise, opens an editor to
create the value; without a terminal it fails instead of opening one.

If the {item} is already staged for creation, edits the staged value.
The new {item} will be created in {provider} when you run '{path} apply'.

Use '{path} edit' to modify an existing {item}.
Use '{path} status' to view staged changes.

EXAMPLES:
   {path} add <name>                Open editor to create new {item}
   {path} add <name> <value>        Create new {item} with given value
   {path} add <name> --value-stdin  Create new {item} with the value read from stdin`)
}

// editHelp returns the Description text for the edit command.
func editHelp(cfg CommandConfig) string {
	return renderHelp(cfg, `Modify a {item} value and stage the change.

If value is provided as an argument (or piped in with --value-stdin), uses
that value directly, even when it is empty. Otherwise, opens an editor to
modify the value; without a terminal it fails instead of opening one.

If the {item} is already staged, edits the staged value.
Otherwise, fetches the current value from {provider} and opens it for editing.
Saves the edited value to the staging area (does not immediately apply to {provider}).

Use '{path} delete' to stage a {item} for deletion.
Use '{path} apply' to apply staged changes to {provider}.
Use '{path} status' to view staged changes.

EXAMPLES:
   {path} edit <name>                Open editor to modify {item}
   {path} edit <name> <value>        Set {item} to given value
   {path} edit <name> --value-stdin  Set {item} to the value read from stdin`)
}

// applyHelp returns the Description text for the apply command.
func applyHelp(cfg CommandConfig) string {
	return renderHelp(cfg, `Apply all staged {item} changes to {provider}.

If a {item} name is specified, only that {item}'s staged changes are applied.
Otherwise, all staged {item} changes are applied.

After successful apply, the staged changes are cleared.

Use '{path} status' to view staged changes before applying.

CONFLICT DETECTION:
   Before applying, suve checks for conflicts to prevent lost updates:
   - For new resources: checks if someone else created it after staging
   - For existing resources: checks if it was modified after staging
   Use --ignore-conflicts to force apply despite conflicts.

EXAMPLES:
   {path} apply                      Apply all staged {item} changes (with confirmation)
   {path} apply <name>               Apply only the specified {item}
   {path} apply --yes                Apply without confirmation
   {path} apply --ignore-conflicts   Apply even if {provider} was modified after staging`)
}

// resetHelp returns the Description text for the reset command.
func resetHelp(cfg CommandConfig) string {
	return renderHelp(cfg, `Remove a {item} from staging area or restore to a specific version.

Without a version specifier, the {item} is simply removed from staging.
With a version specifier, the value at that version is fetched and staged.

Use '{path} reset --all' to unstage all {item}s at once.

VERSION SPECIFIERS:
   <name>          Unstage {item} (remove from staging)
   <name>#<ver>    Restore to specific version
   <name>~1        Restore to 1 version ago

EXAMPLES:
   {path} reset <name>              Unstage (remove from staging)
   {path} reset <name>#<ver>        Stage value from specific version
   {path} reset <name>~1            Stage value from previous version
   {path} reset --all               Unstage all {item}s`)
}

// deleteHelp returns the Description text for the delete command.
// Services with delete options (AWS Secrets Manager) expose recovery-window
// details that the others do not.
func deleteHelp(cfg CommandConfig, hasDeleteOptions bool) string {
	if hasDeleteOptions {
		return renderHelp(cfg, `Stage a {item} for deletion.

The {item} will be deleted from {provider} when you run '{path} apply'.
Use '{path} status' to view staged changes.
Use '{path} reset <name>' to unstage.

RECOVERY WINDOW:
   By default, {item}s are scheduled for deletion after a 30-day recovery window.
   During this period, you can restore the {item} using '{service} restore'.
   Use --force for immediate permanent deletion without recovery.

   Minimum: 7 days
   Maximum: 30 days
   Default: 30 days

EXAMPLES:
   {path} delete <name>                      Stage with 30-day recovery
   {path} delete --recovery-window 7 <name>  Stage with 7-day recovery
   {path} delete --force <name>              Stage for immediate deletion`)
	}

	return renderHelp(cfg, `Stage a {item} for deletion.

The {item} will be deleted from {provider} when you run '{path} apply'.
Use '{path} status' to view staged changes.
Use '{path} reset <name>' to unstage.

EXAMPLES:
   {path} delete <name>  Stage {item} for deletion`)
}

// tagHelp returns the Description text for the tag command.
func tagHelp(cfg CommandConfig) string {
	return renderHelp(cfg, `Stage tags to add or update for a {item}.

Tags are staged locally and applied when you run '{path} apply'.
If the {item} is not already staged, a tag-only change is created.

Use '{path} untag' to stage tag removals.
Use '{path} status' to view staged changes.

EXAMPLES:
   {path} tag <name> env=prod              Stage single tag
   {path} tag <name> env=prod team=api     Stage multiple tags`)
}

// untagHelp returns the Description text for the untag command.
func untagHelp(cfg CommandConfig) string {
	return renderHelp(cfg, `Stage tags to remove from a {item}.

Tag removals are staged locally and applied when you run '{path} apply'.
If the {item} is not already staged, a tag-only change is created.

Use '{path} tag' to stage tag additions.
Use '{path} status' to view staged changes.

EXAMPLES:
   {path} untag <name> env              Stage single tag removal
   {path} untag <name> env team         Stage multiple tag removals`)
}
