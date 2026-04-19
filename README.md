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
global:
  title: Estro           # browser tab title and page heading
  subtitle: My home      # shown below the title; omit to hide
  hostname: 0.0.0.0      # bind address; 0.0.0.0 = all interfaces
  port: 3000
  secret: changeme       # session secret; random per restart if omitted
  # The fields below cascade: global → section → service (most specific wins)
  timeout: 60            # command timeout in seconds
  confirm: true          # ask for confirmation before running
  allowed: [admins]      # who can use the app; omit or null = public
  collapsable: true      # false = section is always open, no collapse chevron
  remote: server.local   # run all commands on this host by default
  columns: 3             # buttons per row on desktop (≥1024px); tablet caps at 2, mobile = 1

users:
  alice:
    password: '$2y$10$...'    # bcrypt hash — see below for how to generate
    groups: [admins, family]  # optional; group names can be used in `allowed`

sections:
  - title: My Section
    allowed: [admins, alice]  # usernames or group names; omit = public
    collapsable: false        # always open
    columns: 2
    timeout: 30               # overrides global for all services in this section
    confirm: false
    remote: user@host         # single host, or a jump chain (see SSH below)
    services:
      - title: My Button
        command: echo hello   # single command, or a list joined with &&
        command:              # multi-step form:
          - echo step 1
          - echo step 2
        allowed: []           # explicit [] = public, overrides any parent restriction
        timeout: 10
        confirm: true
        remote: other-host
```

To generate a password hash:
```sh
docker run --rm httpd htpasswd -bnBC 10 "" YOUR_PASS | tr -d ':\n'; echo
```
</details>

### SSH

Add `remote: user@host` to run a command over SSH. For multi-hop access use a chain — each machine connects to the next using its own `~/.ssh` keys:

```yaml
remote: server.local                      # direct
remote: [jump-host, target]               # via jump-host
remote: [hop1, hop2, target]              # two hops
```

Mount your local SSH keys into the container so the first hop can authenticate:

```yaml
volumes:
  - ~/.ssh:/home/node/.ssh:ro
```

Host key checking is disabled (`StrictHostKeyChecking=no`) — suitable for home network use.
