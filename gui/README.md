# suve GUI

Desktop GUI for suve built with [Wails](https://wails.io/) + Svelte.

## Development

```bash
# From project root
make gui-dev
```

This starts the Wails development server with hot reload at http://localhost:34115.

## Building

```bash
# Build production binary with GUI support
go build -tags production -o bin/suve ./cmd/suve

# Or use wails directly
cd gui && wails build
```

## Architecture

```
gui/
├── main.go              # Wails app entry point
├── app.go               # Go backend (bindings exposed to frontend)
└── build/               # Build assets (icons, Info.plist, etc.)

internal/gui/frontend/
├── src/
│   ├── App.svelte       # Main app component with navigation
│   └── lib/
│       ├── ParamView.svelte    # SSM Parameter Store view
│       ├── SecretView.svelte   # Secrets Manager view
│       └── StagingView.svelte  # Staging workflow view
└── wailsjs/             # Auto-generated Go bindings
```

## Testing

```bash
# Run Playwright tests
cd internal/gui/frontend && npm test

# Record GUI demo
./demo/gui-record.sh
```
