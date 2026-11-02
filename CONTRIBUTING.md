# Contributing

Bug reports, router drivers, translations and fixes are welcome.

```sh
make test      # go vet + go test -race ./...
make demo      # try the UI with invented data
```

## Adding a router

A router type is a **driver**: a package under `internal/drivers/` with a type that implements `router.Driver`
(see `internal/router/router.go`).

```go
type Driver interface {
	Poll(light bool) (*Result, error) // light = only what the traffic graph needs
	Reboot(dry bool) error            // dry = only verify the login
	Alive() bool                      // does the router answer at all?
	Interval() time.Duration          // minimum time between polls while someone watches
	Close()
}
```

1. Create `internal/drivers/<vendor>/` with a `New(router.Credentials) router.Driver`. Fill only what the router
   can tell: `router.Snapshot` (clients, router info, WAN, SSIDs, and optionally `System`), plus **either**
   `Result.Counters` (cumulative WAN bytes) **or** `Result.Speed` (current WAN speed). Everything left empty is
   hidden by the UI. Return `*apperr.Error` values (see `internal/apperr`) for failures the user should see
   translated, and use `internal/netutil` for the HTTP transport and "unreachable" errors.
2. Register it in `internal/drivers/drivers.go` (type constant, label, default address and username, `New`).
3. Add tests. Capture real responses from your router, **anonymize them completely** (MAC addresses, IPs, names,
   SSIDs, serial numbers, keys, tokens) and put them in the package's `testdata/`. `internal/testrouter` has fake
   routers that speak the real login flows; the GL.iNet driver's tests show how to serve fixtures through one.
4. Add the router to the table in `README.md` with what it can and cannot report.

Dependencies point downwards (see "How it works" in the README): a driver may import `router`, `apperr`,
`netutil` and `unixcrypt`, but never `config`, `collector` or `server`.

Use only the Go standard library; the project has no third-party dependencies on purpose.

## Translations

See the "Translations" section of the README. Run the app, switch the language in Settings, and look through every
screen, including the dialogs and the error banners.

## Style

`gofmt`, small functions, comments that explain why. Logs and error messages are English; the UI translates them.
