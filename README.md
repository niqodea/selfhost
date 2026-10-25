# selfhost

A modular Docker Compose setup for self-hosting services on a personal server.

## Apps

| App                 | Description                                       |
|---------------------|---------------------------------------------------|
| **caddy**           | Reverse proxy with automatic Let's Encrypt SSL    |
| **ddns-updater**    | Dynamic DNS for servers without static IPs        |
| **wireguard-lan**   | VPN server for remote access to local services    |
| **wireguard-wan**   | VPN server with anonymous exit via Gluetun        |
| **immich**          | Photo and video backup                            |
| **jellyfin**        | Media streaming server                            |
| **prowlarr**        | Indexer manager with anonymous exit via Gluetun   |
| **qbittorrent**     | Torrent client with anonymous exit via Gluetun    |
| **vaultwarden**     | Password manager (Bitwarden-compatible)           |
| **radicale**        | CalDAV/CardDAV server for contacts and calendars  |
| **git**             | Self-hosted Git server                            |
| **librespeed**      | Network speed test                                |

**wireguard-wan**, **prowlarr**, and **qbittorrent** route their traffic through the same Gluetun VPN gateway for anonymity.

## Getting Started

1. Clone the repo and create a deployment directory (e.g., `/opt/selfhost`)
2. Install base systemd files: `.host-setup/install-systemd-base`
3. Install deployment: `.host-setup/install-deployment /opt/selfhost`
4. Copy socket samples to create actual sockets (`.~.foo` -> `._.foo`)
5. Download and build the images: `/opt/selfhost/prepare-images`
6. Enable startup service: `.host-setup/configure-deployment-startup /opt/selfhost up`

Startup stops if an image is missing, because it never downloads or builds one.

### Exp

The exp deployment runs from the repo checkout itself, on rootless Docker, which needs `sudo` only once and does not start on boot.
Its config is the checkout's socket files, which are committed, so an edit takes effect on the next start.

1. Install host prerequisites: `sudo .host-setup/install-rootless-base <user>`
2. After every boot, from an SSH session or tmux, as `<user>`:
   - `.exp/start-daemon`
   - `.exp/route-failsafe-up`
   - `.exp/prepare-images`, which downloads and builds the images, only after a clone or a pull
   - `.exp/startup`, which first creates the directory sockets a fresh clone lacks

The daemon stops with `kill "$(cat "$XDG_RUNTIME_DIR/docker.pid")"`, and takes the containers and their routes with it.

To browse exp's apps at `https://<app>.exp.test`, connect a WireGuard client to exp's wireguard-lan, create a Firefox profile that trusts exp's CA once with `.exp/create-firefox-profile`, and open it with `firefox --profile .exp/firefox-profile --new-instance`. Creating the profile needs NSS's `certutil` and OpenSSL 3.4 or later on the `PATH`.

## Project Structure

```
docker-compose.yaml               # Shared networks
.infra/                           # Modules the apps depend on, started before them
├── dns/                          # DNS resolvers with the blocklist
└── gluetun-gateway/              # VPN gateway
route-failsafe                    # Network failsafe routing script
prepare-images                    # Download and build the images
startup                           # Start the services
.env                              # Shared config
<module>/
├── docker-compose.yaml           # Docker resources
├── .env -> ../.env
│
├── Dockerfile                    # Custom image build
├── route                         # Host-level network routing script
├── volumes/                      # Persistent data and configs
└── local.env                     # Container env vars

.host-setup/                      # Installation and configuration scripts
.exp/                             # Scripts that run the checkout as the exp deployment
.adr/                             # Architecture Decision Records
```

## Design Patterns

### Gluetun as Network Gateway

Docker can't natively use a container as a network gateway.
A few iptables rules on the host solve this, routing all traffic from the `gluetun` network through the Gluetun container.

Services opt into VPN routing by joining the network with `gw_priority: 1` and setting `dns` to `GLUETUN_NETWORK_DNS_ADDRESS`.
One Gluetun instance serves all services that need anonymous egress.

Gluetun is in `.infra/gluetun-gateway/`, and startup starts it before the apps. Until the tunnel is up, Gluetun drops the traffic of the apps routed through it.

### DNS Blocklist

Two dnsmasq resolvers in `.infra/dns/` answer every app's DNS queries, with the StevenBlack blocklist built into one image.
`dns-internet` sends the remaining queries out directly, and `dns-gluetun` sends them through Gluetun, so routed apps never resolve names outside the tunnel.
Every app that reaches the internet sets `dns` to its network's resolver, and each WireGuard server gives its peers its side's resolver.
Both resolvers also answer the domain with Caddy's address and `syncthing.internal` with Syncthing's, so Caddy and ddns-updater check their records through a public resolver.

### Volume Ownership

Each volume is one of three kinds, and the kind decides who owns its files:
- **App state** (databases, app configs, caches): a named volume backed by the app's folder in `volumes/`, with a name that starts with the app's name. The app runs as its image's default user.
- **Shared content** (downloads, synced folders, git repositories): a plain bind mount owned by the deployment owner, whose IDs are `PUID` and `PGID`. Only the apps that write there run as those IDs.
- **Config you write** (Caddyfile, templates): a read-only bind mount.

See [the ADR](.adr/volume-ownership.md) for why.

### Plugs

File dependencies are declared using plugs, a two-layer symlink system that makes external dependencies explicit:
- **Plug** (committed): `local.env -> ._.local.env` points to the socket
- **Socket** (gitignored, except the checkout's file sockets, which are the exp deployment's config): `._.local.env` contains or links to the actual file, and `.=.data` is or links to the actual directory
- **Sample** (created by `install-deployment` from the checkout's socket): `.~.local.env` shows the expected structure, with exp's placeholder values

Bootstrap by copying the sample: `cp .~.local.env ._.local.env`

This makes dependencies visible in version control without committing environment-specific paths or secrets.
Learn more at the [plugs repo](https://github.com/niqodea/plugs).

### Breadcrumbs

Breadcrumbs are a navigation pattern using specially-named symlinks (prefixed with `..`) that create a trail back to target directories.
Each breadcrumb points either further up the chain (`../..foo`) or to the current directory (`.`) when the target is reached.

```
./caddy/..repo  ->  ../..repo  (points up one level)
./..repo        ->  .          (marks arrival at ./)
```

This approach provides more explicit and self-documenting navigation than using bare `../` in symlinks: the breadcrumb name tells you _what_ you're pointing to, not just _where_.
Learn more at the [breadcrumbs repo](https://github.com/niqodea/breadcrumbs).

## License

MIT
