# selfhost

A modular Docker Compose setup for self-hosting services on a personal server.

## Apps

| App                 | Description                                       |
|---------------------|---------------------------------------------------|
| **caddy**           | Reverse proxy with automatic Let's Encrypt SSL    |
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
2. Install base systemd files: `host-setup/install-systemd-base`
3. Install deployment: `host-setup/install-deployment /opt/selfhost`
4. Copy socket samples to create actual sockets (`.~.foo` -> `._.foo`), and plug each app's `content/` as the deployment owner, with an empty directory or a link to your data
5. Download and build the images: `/opt/selfhost/prepare-images`
6. Enable startup service: `host-setup/configure-deployment-startup /opt/selfhost up`

Startup stops if an image is missing, because it never downloads or builds one.

### Exp

The exp deployment runs in a Lima VM, with fakes in place of the VPN provider, Dynu and Let's Encrypt.
Its config is the samples, as they are.

1. Once per machine, install QEMU, then log in again: `vm/install-qemu`
2. Create the VM once: `vm/manage create`
3. Copy the repo to `~/selfh-source` in the VM: `vm/manage push`
4. In the VM (`limactl shell selfh`):
   - Once per VM: `sudo ~/selfh-source/host-setup/install-systemd-base`
   - `sudo ~/selfh-source/exp/install-deployment /opt/selfh-exp`, which also plugs the samples and starts the fakes
   - `sudo /opt/selfh-exp/prepare-images`
   - `sudo ~/selfh-source/host-setup/configure-deployment-startup /opt/selfh-exp up`
5. In the VM, check that exp keeps each promise: `for check in /opt/selfh-exp/exp/checks/*; do sudo "$check"; done`

## Project Structure

```
docker-compose.yaml               # Shared networks
.env                              # Shared config
route-failsafe                    # Network failsafe routing script
prepare-images                    # Download and build the images
startup                           # Start the services
shutdown                          # Stop the services
infra/                            # Network modules, started before the apps
├── ddns-updater/                 # Points the domain at the public IP
├── dns/                          # DNS resolvers with the blocklist
└── gluetun-gateway/              # VPN gateway
proxies/                          # Images of the proxies through which apps reach the internet
├── http/                         # HTTPS, only to the domains the app lists
└── dns/                          # DNS, only for the names the app forwards
apps/
└── <app>/
    ├── docker-compose.yaml       # Docker resources
    ├── .env -> ..repo/.env
    │
    ├── Dockerfile                # Custom image build
    ├── route                     # Host-level network routing script
    ├── volumes/                  # App state and configs
    ├── content/                  # Your data, which the app works on
    └── local.env                 # Container env vars

host-setup/                       # Installation and configuration scripts
exp/                              # The exp deployment's fakes and scripts
vm/                               # The Lima VM that exp runs in
adr/                              # Architecture Decision Records
```

## Design Patterns

### Gluetun as Network Gateway

Docker can't natively use a container as a network gateway.
A few iptables rules on the host solve this, routing all traffic from the `gluetun` network through the Gluetun container.

Services opt into VPN routing by joining the network with `gw_priority: 1` and setting `dns` to `GLUETUN_NETWORK_DNS_ADDRESS`.
One Gluetun instance serves all services that need anonymous egress.

Gluetun is in `infra/gluetun-gateway/`, and startup starts it before the apps. Until the tunnel is up, Gluetun drops the traffic of the apps routed through it.
Gluetun refreshes its server list only through the tunnel, so `gluetun-servers-updater` refreshes it from the provider each time Gluetun starts.

### Internet Access Through Proxies

No app joins the `internet` network: only the infra, the WireGuard servers and the proxies do.
An app that needs the internet runs its own proxies, named `<app>-http-proxy` and `<app>-dns-proxy`, built from `proxies/` and joined to the app's private network:
- `http-proxy` allows HTTPS only to the domains in the app's `volumes/http-proxy-domains.txt`.
- `dns-proxy` answers only the names that the app's compose file gives it with `--server`, for apps that look up DNS records themselves.

The HTTP proxy logs each domain it refuses, so letting one through means adding it to the list and restarting the proxy.
The proxy of an app routed through Gluetun joins `gluetun` instead of `internet`.

### DNS Blocklist

Two dnsmasq resolvers in `infra/dns/` answer every app's DNS queries, with the StevenBlack blocklist built into one image.
`dns-internet` sends the remaining queries out directly, and `dns-gluetun` sends them through Gluetun, so routed apps never resolve names outside the tunnel.
Every container on `internet` or `gluetun` sets `dns` to its network's resolver, and each WireGuard server gives its peers its side's resolver.
Both resolvers also answer the domain with Caddy's address and `syncthing.internal` with Syncthing's, so Caddy checks its records through a public resolver.

### Volume Ownership

Each volume is one of three kinds, and the kind decides who owns its files:
- **App state** (databases, app configs, caches): a named volume backed by the app's folder in `volumes/`, with a name that starts with the app's name. The app runs as its image's default user.
- **Shared content** (downloads, synced folders, git repositories, media): a plain bind mount of the app's folder in `content/`, owned by the deployment owner, whose IDs are `PUID` and `PGID`. Only the apps that write there run as those IDs.
- **Config you write** (templates): a read-only bind mount.

The user an app runs as owns its state, and each app runs as one of three:
- **Root**: caddy, gluetun-gateway, immich, jellyfin and vaultwarden.
- **The deployment owner**: git, qbittorrent and syncthing, which write shared content.
- **The image's own user**: immich-postgres (999), radicale (2999), prowlarr (911), and wireguard-lan and wireguard-wan (911).

See [the ADR](adr/volume-ownership.md) for why.

### Plugs

File dependencies are declared using plugs, a two-layer symlink system that makes external dependencies explicit:
- **Plug** (committed): `local.env -> ._.local.env` points to the socket
- **Socket** (gitignored): `._.local.env` contains or links to the actual file, and `.=.data` is or links to the actual directory
- **Sample** (committed): `.~.local.env` shows the expected structure, and its values are exp's config

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
