# gomatrixserverlib

[![GoDoc](https://godoc.org/codefloe.com/pat-s/gomatrixserverlib?status.svg)](https://godoc.org/codefloe.com/pat-s/gomatrixserverlib)

A fork of [matrix-org/gomatrixserverlib](https://github.com/matrix-org/gomatrixserverlib) — a Go library for [Matrix](https://matrix.org/) protocol server implementations.
Requires Go 1.25+.

This fork replaces archived and stale upstream dependencies with actively maintained alternatives and local implementations, and fixes various bugs.

Provides event creation, parsing, and validation; cryptographic signing; state resolution algorithms (v1 and v2); and a federation client for server-to-server communication.
Used as a dependency by Matrix homeservers like [Dendrite](https://github.com/element-hq/dendrite).

## Packages

| Package                    | Description                                                                                                                                |
| -------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| `gomatrixserverlib` (root) | Core event handling, authentication, state resolution, federation join/invite/leave operations, and cryptographic signing                  |
| `fclient/`                 | Federation HTTP client: server-to-server communication, DNS/well-known resolution, DNS caching, invite protocols                           |
| `spec/`                    | Type-safe wrappers for Matrix protocol identifiers (`UserID`, `RoomID`, `SenderID`, `ServerName`) and constants (event types, error codes) |
| `tokens/`                  | Macaroon-based authentication token generation and validation                                                                              |

## Testing

```sh
# Run all tests
go test -race ./...

# Run a single test
go test -race -run TestName ./...

# Lint
golangci-lint run
```
