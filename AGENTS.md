# Agents Guide

This project is written in Go and uses Go modules. The module's go version is 1.24, so install Go 1.24 or newer.

## Setup

1. Install Go 1.24 or later.
2. Download dependencies with:

```sh
go mod download
```

## Testing

No test suite is provided. Do not add one.

## Formatting

Format source code with:

```sh
go fmt ./...
```

## Code Quality

Run vet before committing:

```sh
go vet ./...
```

You may also run `golangci-lint` if available:

```sh
golangci-lint run ./...
```

## Building

Build the project with:

```sh
go build ./...
```

## Comments

Keep comments short and only when they clarify non-obvious logic.
