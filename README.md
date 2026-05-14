# Estro

A minimal web UI for running shell commands from the browser — local or over SSH.

## Run

<details><summary>Docker</summary>
<p></p>

See [`compose.yaml`](compose.yaml) file. Then:

```sh
docker compose up -d
```
</details>

<details><summary>Go</summary>
<p></p>

```sh
go build -o estro ./cmd/estro
./estro -config config.yaml
```

Or run directly:

```sh
go run ./cmd/estro -config config.yaml
```

The `-config` flag can also be set via the `ESTRO_CONFIG` environment variable.
</details>

## Configuration

All configuration lives in a single `config.yaml` file.

<details><summary>Configuration reference</summary>
<p></p>

```yaml
---
global:                  # optional; all sub-fields optional
  title: Estro           # browser tab title and page heading (default: 'Estro')
  subtitle: My home      # shown below the title; omit to hide (default: hidden)
  hostname: 0.0.0.0      # bind address; 0.0.0.0 = all interfaces (default: 127.0.0.1)
  port: 3000             # (default: 3000)
  secret: changeme       # session secret; random per restart if omitted
  # The fields below cascade: global → section → service (most specific wins)
  allowed: [admins]      # who can use the app; null / [] = public (default: public)
  timeout: 60            # command timeout in seconds (default: 60)
  confirm: true          # ask for confirmation before running (default: true)
  remote: server.local   # run all commands on this host (default: run locally)
  # remote: user@host             # another option
  # remote: [server.local]        # <=> remote: server.local
  # remote: [jump-host, target]   # run on target via jump-host
  # remote: [hop1, hop2, target]  # two hops
  collapsable: true      # false = section is always open, no collapse chevron (default: true)
  columns: 3             # buttons per row on desktop (≥1024px); tablet caps at 2, mobile = 1 (default: 3)

users:                   # optional; omit for no authentication
  alice:
    password: '$2y$10$...'    # required; bcrypt hash — see below for how to generate
    groups: [admins, family]  # optional; group names can be used in `allowed`

sections:                # optional; omit for no buttons
  - title: My Section    # required
    allowed: [admins, alice]  # optional; overrides global.allowed
    timeout: 30               # optional; overrides global.timeout
    confirm: false            # optional; overrides global.confirm
    remote: user@host         # optional; overrides global.remote
    collapsable: false        # optional; overrides global.collapsable
    columns: 2                # optional; overrides global.columns
    services:
      - title: My Button      # required
        command:              # required; multi-step (list of commands joined with &&)
          - echo step 1
          - echo step 2
        # command: echo hello   # single command
        allowed: []           # optional; overrides section's allowed
        timeout: 10           # optional; overrides section's timeout
        confirm: true         # optional; overrides section's confirm
        remote: other-host    # optional; overrides section's remote
```

To generate a password hash:
```sh
docker run --rm httpd htpasswd -bnBC 10 "" YOUR_PASS | tr -d ':\n'; echo
```
</details>

## Security

**Estro is designed for trusted home networks only. Do not expose it to the internet.**

- Sessions use `httpOnly`, `sameSite=strict` cookies
- Passwords are stored as bcrypt hashes (cost 10)
- Login attempts are rate-limited to 10 per 15 minutes per IP
- `StrictHostKeyChecking=no` means SSH connections are vulnerable to MITM on untrusted networks — only use on LANs you control

In general it's just a tiny pet project for my home labbing, for now with ~0 stability and no clear idea of how to make it some better than just to serve my needs. 

### Access control: `allowed: []` vs `allowed: null`

Both `allowed: null` (or omitting the field) and `allowed: []` (an empty list) result in a **public** service. An empty list does **not** mean "nobody" — it means no restriction. To restrict access, provide at least one username or group name.

### Persistent session secret

If no `secret` is set in `config.yaml`, a random secret is generated on every restart, invalidating all existing sessions. Set a stable secret for persistent logins:

```yaml
global:
  secret: your-random-secret-here
```

## Development

```sh
go run ./cmd/estro -config config.yaml          # run with auto-recompile
go test -race ./...                             # run tests
go test -race -coverprofile=coverage.out ./...  # tests with coverage
```