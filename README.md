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

<details><summary>Node</summary>
<p></p>

```sh
npm install
npm start        # production
npm run dev      # watch mode
```
</details>

## Configuration

All configuration lives in a single `config.yaml` file.

<details><summary>Configuration reference</summary>
<p></p>

```yaml
# Global config
config:
  title: Estro          # (Optional) Page title; defaults to 'Estro'
  subtitle: string      # (Optional) Subtitle shown below title; omit to hide
  hostname: 127.0.0.1   # (Optional) Bind address; defaults to 127.0.0.1
  port: 3000            # (Optional) Port; defaults to 3000
  timeout: 30           # (Optional) Default command timeout in seconds; defaults to 60
  # secret: string      # (Optional) Session secret; random per restart if omitted

# Users
users:
  alice:
    # Generated password with
    #   docker run --rm httpd htpasswd -bnBC 10 "" YOUR_PASS | tr -d ':\n'; echo
    password: '$2y$10$...'   # (Required) bcrypt hash
    groups: [admins, family] # (Optional) Group memberships for use in `allowed`

# Services sections
sections:                         # Top-level list of sections
  - title: string                 # (Required) Section heading
    allowed: [user, group, ...]   # (Optional) Restrict to these usernames/groups; omit for public
    expanded: false               # (Optional) true = always open, collapse disabled
    services:                     # List of buttons in this section
      - title: string             # (Required) Button label
        command: string|list      # (Required) Shell command — string or list of strings joined with &&
        remote: user@host         # (Optional) Run over SSH instead of locally
        timeout: 30               # (Optional) Timeout in seconds; overrides global default
        confirm: true             # (Optional) Show confirmation dialog before running
        allowed: [user, group]    # (Optional) Overrides section-level allowed for this service
```

`allowed` values can be usernames or group names - groups are expanded from `users[].groups`. Omitting `allowed` on both section and service makes the service public.
</details>

### SSH

Mount your SSH keys into the container:

```yaml
volumes:
  - ~/.ssh:/home/node/.ssh:ro
```

Host key checking is disabled (`StrictHostKeyChecking=no`) — suitable for home network use.
