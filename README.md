# Estro

A minimal web UI for running shell commands from the browser — local or over SSH.

## Run with Docker

See [`compose.yaml`](compose.yaml) file. Then:

```sh
docker compose up -d
```

## Run without Docker

Requires Node.js ≥ 18.

```sh
npm install
npm start        # production
npm run dev      # watch mode
```

## Configuration

All configuration lives in a single `config.yaml` file.

### Global config

```yaml
config:
  title: Estro          # page title — defaults to 'Estro'
  subtitle: ""          # subtitle shown below title
  hostname: 127.0.0.1   # bind address
  port: 3000
  timeout: 30           # default command timeout in seconds
  secret: ""            # session secret — random per restart if omitted
```

### Users

Passwords are bcrypt hashes. Generate one with:

```sh
docker run --rm httpd htpasswd -bnBC 10 "" yourpassword | tr -d ':\n'; echo
```

```yaml
users:
  alice:
    password: '$2y$10$...'
    groups: [admins, family]  # optional
  bob:
    password: '$2y$10$...'
```

### Sections and services

```yaml
sections:
  - title: My Section
    allowed: [alice, admins]  # usernames or group names; omit for public
    expanded: true            # always open, collapse disabled — defaults to false
    services:
      - title: Say hello
        command: echo hello

      - title: Multi-step
        command:
          - cd /tmp
          - ls -1

      - title: Remote command
        command: uptime
        remote: user@host     # runs over SSH
        timeout: 60           # per-service override in seconds
        confirm: false        # skip confirmation dialog — defaults to true
        allowed: [bob]        # overrides section-level allowed
```

### Access control

- `allowed` on a section applies to all its services unless overridden on the service.
- Values in `allowed` can be usernames or group names — groups are resolved from `users[].groups`.
- Omitting `allowed` entirely makes the service public (no login required).

### SSH

Mount your SSH keys into the container and set the `remote` field on a service:

```yaml
volumes:
  - ~/.ssh:/home/node/.ssh:ro
```

Host key checking is disabled (`StrictHostKeyChecking=no`) — suitable for home network use.
