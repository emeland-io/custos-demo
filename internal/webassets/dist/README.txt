This directory is intentionally committed as a placeholder.

Go's `//go:embed all:dist` directive requires this directory to exist
and contain at least one tracked, non-hidden file at compile time, or
`go build` fails for anyone who hasn't built the frontend yet.

Running `make frontend-dist` (or the Docker build) overwrites every
file in this directory, including this one, with the real built Vue
app. Local, uncommitted changes made by a frontend build are ignored
by git (see .gitignore) except for this file, which git always tracks
so a fresh clone keeps building.
