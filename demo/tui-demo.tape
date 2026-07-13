# TUI Demo Recording for suve
# This tape reproduces the CLI/GUI demo scenario in the keyboard-driven TUI:
# browse, stage (new / edit / tag add / tag remove / delete), review, apply all,
# then verify — on AWS Parameter Store via LocalStack, over the /demo/* namespace.
#
# Recorded with VHS: https://github.com/charmbracelet/vhs
# Run: ./demo/tui-record.sh   (seeds LocalStack, then runs `vhs demo/tui-demo.tape`)

Output "demo/tui-demo.gif"

Set FontSize 16
Set Width 1400
Set Height 800
Set Theme "Dracula"
Set Padding 20

# Configure LocalStack endpoint (tui-record.sh rewrites the port when
# SUVE_LOCALSTACK_EXTERNAL_PORT is set; SUVE_STAGING_KEY is inherited from it).
Env AWS_ENDPOINT_URL "http://localhost:4566"
Env AWS_ACCESS_KEY_ID "test"
Env AWS_SECRET_ACCESS_KEY "test"
Env AWS_DEFAULT_REGION "us-east-1"

# Launch the TUI. With only LocalStack AWS env active, AWS is the sole active
# provider, so `suve --tui` opens straight on the AWS Param tab.
Type "suve --tui"
Enter
Sleep 4s

# ===========================================================================
# 1. Browse: reveal values in the list; the first entry (/demo/api/url) detail
#    shows its value and the Version=v1 tag.
# ===========================================================================
Type "v"
Sleep 3s

# ===========================================================================
# 2. Stage a NEW parameter: /demo/cache/ttl = 3600
#    (n → Name → Type[String] → Value → [ OK ] → Stage)
# ===========================================================================
Type "n"
Sleep 2s
Type "/demo/cache/ttl"
Sleep 1s
Enter
Sleep 800ms
Enter
Sleep 800ms
Type "3600"
Sleep 1.5s
Tab
Sleep 800ms
Tab
Sleep 800ms
Enter
Sleep 1.5s
Enter
Sleep 2.5s

# ===========================================================================
# 4. Stage an EDIT of /demo/api/url: value v1 -> v2
#    (e → Type -> Value; backspace "1.example.com", type "2.example.com" → Stage)
# ===========================================================================
Type "e"
Sleep 2s
Enter
Sleep 1s
Backspace 13
Sleep 1s
Type "2.example.com"
Sleep 1.5s
Tab
Sleep 800ms
Tab
Sleep 800ms
Enter
Sleep 1.5s
Enter
Sleep 2.5s

# ===========================================================================
# 5. Stage tag Env=Dev on /demo/api/url
#    (t → Action[Add] → Key=Env → Value=Dev → Stage)
# ===========================================================================
Type "t"
Sleep 2s
Enter
Sleep 800ms
Type "Env"
Sleep 800ms
Enter
Sleep 800ms
Type "Dev"
Sleep 1s
Enter
Sleep 1.5s
Enter
Sleep 2.5s

# ===========================================================================
# 6. Stage REMOVAL of the Version tag on /demo/api/url
#    (t → Action[Remove] → Tag[Version=v1] → Stage)
# ===========================================================================
Type "t"
Sleep 2s
Right
Sleep 1.5s
Enter
Sleep 1s
Enter
Sleep 1.5s
Enter
Sleep 2.5s

# ===========================================================================
# 7. Stage DELETION of /demo/legacy/endpoint
#    (down to it → d → [ Delete ] → Stage)
# ===========================================================================
Down
Sleep 800ms
Down
Sleep 1.5s
Type "d"
Sleep 2s
Enter
Sleep 1.5s
Enter
Sleep 2.5s

# ===========================================================================
# 8/3. Switch to the Staging tab; stage tag Env=Dev on the new /demo/cache/ttl
#      (only reachable here — a not-yet-applied create is not in the browser
#      list), then review the changes in both value and diff views.
# ===========================================================================
Type "3"
Sleep 3s
Down
Sleep 1.5s
Type "t"
Sleep 2s
Type "Env"
Sleep 800ms
Enter
Sleep 800ms
Type "Dev"
Sleep 1s
Enter
Sleep 2.5s
# Toggle to raw value view, then back to the remote-vs-staged diff view.
Type "v"
Sleep 3s
Type "v"
Sleep 2.5s

# ===========================================================================
# 9. Apply ALL staged changes (A → focus Apply → Enter), then show the results.
# ===========================================================================
Type "A"
Sleep 2s
Down
Sleep 1s
Enter
Sleep 3.5s
Enter
Sleep 2s

# ===========================================================================
# 10. Back on the Param tab, verify the applied state with values revealed.
#     (1 rebuilds/reloads the tab; /demo/api/url is auto-selected, now v2 with
#      the Env=Dev tag and no Version tag; /demo/cache/ttl exists; legacy gone.)
# ===========================================================================
Type "1"
Sleep 3s
Type "v"
Sleep 3s

# ===========================================================================
# 11. Show /demo/api/url's version history (enter focuses the history pane:
#     per-version values, #2 = current v2, #1 = the old v1).
# ===========================================================================
Enter
Sleep 3.5s

# Quit cleanly from the browser.
Escape
Sleep 800ms
Type "q"
Sleep 1.5s
