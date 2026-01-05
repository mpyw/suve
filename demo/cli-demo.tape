# CLI Demo Recording for suve
# This tape demonstrates staging workflow (Git-like staging for AWS)
#
# Recorded with VHS: https://github.com/charmbracelet/vhs
# Run: vhs demo/cli-demo.tape

Output demo/cli-demo.gif

Set FontSize 16
Set Width 1200
Set Height 700
Set Theme "Dracula"
Set Padding 20

# Configure LocalStack endpoint
Env AWS_ENDPOINT_URL "http://localhost:4566"
Env AWS_ACCESS_KEY_ID "test"
Env AWS_SECRET_ACCESS_KEY "test"
Env AWS_DEFAULT_REGION "us-east-1"

Type "# suve - Git-like CLI for AWS Parameter Store"
Enter
Sleep 1s

Type "# List existing parameters"
Enter
Sleep 500ms

Type "suve param ls -R /demo"
Enter
Sleep 2s
Enter
Sleep 500ms

Type "# Stage a new parameter"
Enter
Sleep 500ms

Type "suve stage param add /demo/cache/ttl 3600"
Enter
Sleep 1.5s
Enter
Sleep 500ms

Type "# Add tag to the new parameter"
Enter
Sleep 500ms

Type "suve stage param tag /demo/cache/ttl Env=Dev"
Enter
Sleep 1.5s
Enter
Sleep 500ms

Type "# Stage an update to existing parameter"
Enter
Sleep 500ms

Type "suve stage param edit /demo/api/url https://api-v2.example.com"
Enter
Sleep 1.5s
Enter
Sleep 500ms

Type "# Add tag to the edited parameter"
Enter
Sleep 500ms

Type "suve stage param tag /demo/api/url Env=Dev"
Enter
Sleep 1.5s
Enter
Sleep 500ms

Type "# Remove old tag from the edited parameter"
Enter
Sleep 500ms

Type "suve stage param untag /demo/api/url Version"
Enter
Sleep 1.5s
Enter
Sleep 500ms

Type "# Stage a deletion"
Enter
Sleep 500ms

Type "suve stage param delete /demo/legacy/endpoint"
Enter
Sleep 1.5s
Enter
Sleep 500ms

Type "# Check staging status"
Enter
Sleep 500ms

Type "suve stage status"
Enter
Sleep 2.5s
Enter
Sleep 500ms

Type "# Preview changes (like git diff --staged)"
Enter
Sleep 500ms

Type "suve stage diff"
Enter
Sleep 3.5s
Enter
Sleep 500ms

Type "# Apply all staged changes"
Enter
Sleep 500ms

Type "suve stage apply"
Enter
Sleep 1s
Type "y"
Enter
Sleep 2.5s
Enter
Sleep 500ms

Type "# Verify: list parameters with tags"
Enter
Sleep 500ms

Type "suve param ls -R /demo --show"
Enter
Sleep 2s
Enter
Sleep 500ms

Type "# Check version history with patch"
Enter
Sleep 500ms

Type "suve param log -p /demo/api/url"
Enter
Sleep 3.5s
