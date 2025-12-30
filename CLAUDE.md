# CLAUDE.md - suve CLI Redesign Document

## Project Overview

Redesigning "suve" - a CLI tool for AWS Parameter Store and Secrets Manager operations.
Goal: Git-like user experience.

## Design Philosophy

### 1. Git-like Command Structure

```
# Basic structure
suve <service> <command> [options] [arguments]

# Services
- ssm (or ps/param): Parameter Store
- sm (or secret): Secrets Manager
```

### 2. Command Mapping (git -> suve)

| git command | suve equivalent | Description |
|-------------|-----------------|-------------|
| git show | suve show | Display value |
| git log | suve log | Show history |
| git diff | suve diff | Show diff between versions |
| git add + commit | suve set | Set value (create/update) |
| git rm | suve rm | Delete |
| git ls-files | suve ls | List items |
| git checkout | suve restore | Restore deleted item |
| git cat-file | suve cat | Raw output (for piping) |

### 3. Version Specification (Git-like revision syntax)

```bash
# Latest
suve ssm show /my/param

# Specific version (Parameter Store uses numbers, Secrets Manager uses UUIDs)
suve ssm show /my/param@3
suve sm show my-secret@abc123-...

# N versions ago (like git's HEAD~N)
suve ssm show /my/param~1    # 1 version ago
suve ssm show /my/param~2    # 2 versions ago

# Staging labels (Secrets Manager only, like git branches/tags)
suve sm show my-secret:AWSCURRENT
suve sm show my-secret:AWSPREVIOUS
```

## Command Reference

### Parameter Store (ssm)

```bash
# Show value
suve ssm show <name[@version][~shift]>
suve ssm show /my/param
suve ssm show /my/param@3
suve ssm show /my/param~1

# Raw value output (for piping/redirection)
suve ssm cat <name[@version][~shift]>

# Show history
suve ssm log <name> [-n <count>]

# Show diff
suve ssm diff <name> <version1> [version2]

# List parameters
suve ssm ls [path-prefix] [--recursive/-r]

# Set value
suve ssm set <name> <value> [--type <type>]

# Delete
suve ssm rm <name>
```

### Secrets Manager (sm)

```bash
# Show value
suve sm show <name[@version][~shift][:label]>

# Raw value output
suve sm cat <name[@version][~shift][:label]>

# Show history
suve sm log <name> [-n <count>]

# Show diff
suve sm diff <name> <version1> [version2]

# List secrets
suve sm ls [--filter <prefix>]

# Create new secret
suve sm create <name> <value> [--description <desc>]

# Update secret value
suve sm set <name> <value>

# Delete
suve sm rm <name> [--force] [--recovery-window <days>]

# Restore
suve sm restore <name>
```

## Architecture

```
cmd/
  suve/
    main.go         # Entry point (go install ready)

internal/
  cli/
    app.go          # CLI app definition
    ssm.go          # Parameter Store commands
    sm.go           # Secrets Manager commands

  aws/
    client.go       # AWS client initialization

  version/
    spec.go         # Version specification parsing (@N, ~N, :LABEL)

  output/
    output.go       # Output formatting (diff, colors, fields)

bin/                # Build output (gitignored)
```

## Version Specification Grammar

```
<name>[@<version>][~<shift>][:label]

Examples:
/my/param           # Latest
/my/param@3         # Version 3
/my/param~1         # 1 version ago
/my/param@5~2       # 2 versions before version 5 (= version 3)
my-secret:AWSCURRENT
my-secret:AWSPREVIOUS
```

## Output Formats

### show command
```
Name: /my/parameter
Version: 3
Type: SecureString
Modified: 2024-01-15T10:30:45Z

  my-secret-value
```

### log command
```
Version 3 (current)
Date: 2024-01-15T10:30:45Z
User: arn:aws:iam::...
my-secret-value...

Version 2
Date: 2024-01-14T09:20:30Z
...
```

### diff command
```diff
--- /my/param@2
+++ /my/param@3
@@ -1 +1 @@
-old-value
+new-value
```

## Implementation Status

### Completed
- [x] Project structure (cmd/, internal/)
- [x] Version specification parser (@N, ~N, :LABEL)
- [x] AWS client initialization
- [x] urfave/cli v2 command definitions
- [x] SSM commands: show, cat, log, diff, ls, set, rm
- [x] SM commands: show, cat, log, diff, ls, create, set, rm, restore
- [x] Colored output with diff highlighting

### Build

```bash
# Build to bin/
go build -o bin/suve ./cmd/suve

# Install globally
go install ./cmd/suve
```

## Files

| File | Purpose |
|------|---------|
| cmd/suve/main.go | Entry point |
| internal/cli/app.go | CLI app definition |
| internal/cli/ssm.go | Parameter Store commands |
| internal/cli/sm.go | Secrets Manager commands |
| internal/version/spec.go | Version spec parser |
| internal/aws/client.go | AWS SDK client wrapper |
| internal/output/output.go | Output formatting utilities |

---

## Work Log

### Session 1 (2024-12-30)
- Analyzed existing code structure
- Created design document
- Implemented from scratch using urfave/cli v2
- Created version parser with @N, ~N, :LABEL support
- Implemented all SSM commands
- Implemented all SM commands
- Build successful

### Session 2 (2024-12-30)
- Restructured for Go distribution (`cmd/suve/main.go`)
- Extracted CLI logic to `internal/cli/`
- Removed unused code (`getParametersByPath`)
- Updated golangci-lint config to v2 format
- Removed Docker-based golangci-lint wrapper
- Added `bin/` directory with .gitignore
