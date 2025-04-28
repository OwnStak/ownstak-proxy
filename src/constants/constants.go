package constants

var (
	// These are default placeholder values for the app name and version in the dev mode.
	// During the build process, these values are overridden with the actual version and app name.
	// See: scripts/build.sh
	AppName    = "OwnStak Proxy DEV"
	Version    = "0.0.0"
	ConsoleURL = "https://console-dev.ownstak.com"
	Mode       = "development" // "development" or "production" (boolean would be better, but linker doesn't seem to support it with -X flag)
	// The prefix all our internal endpoints.
	// E.g. /__ownstak__/health
	InternalPathPrefix = "/__ownstak__"
)
