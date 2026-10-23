# Known limitations

- Simulation only: flat $8.99, no address validation, nothing is actually shipped or stored.
- `ShipOrder` dereferences `req.Address` without a nil check; a request with no address crashes the process (no recovery interceptor).
- pprof is always exposed on the metrics port.
- `amd64`-only image that runs as root unless the pod overrides it.
