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

1. Clone the repo, install `acl`, and create the deployment owner, then join its group. Its user and group ID is 900 plus the deployment's index, like `selfh` with 900 and `selfh-dev` with 901:
   `groupadd --system --gid 900 selfh && useradd --system --uid 900 --gid 900 --no-create-home --shell /usr/sbin/nologin selfh && usermod -aG selfh "$USER"`
2. Install base systemd files: `host-setup/install-systemd-base`
3. Install deployment: `host-setup/install-deployment /opt/selfhost selfh`
4. Copy socket samples to create actual sockets (`.~.foo` -> `._.foo`), with mode 600 for the `.env` ones, and plug each app's `content/` with a directory owned by the deployment owner, or a link to one, made shared with the group: `install -d -o selfh -g selfh -m 2770 <dir> && setfacl -d -m g::rwX <dir>`
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

Every app writes as the deployment owner, a system user named after the Compose project, whose IDs are `PUID` and `PGID`.
It owns all state and content, and you join its group to edit content and read state without `sudo`, except what an app keeps private, like Postgres's data and WireGuard's keys.
- **App state** (databases, app configs, caches): a plain bind mount of the app's directory in `volumes/`.
- **Shared content** (downloads, synced directories, git repositories, media): a plain bind mount of the app's directory in `content/`, whose setgid bit and default ACL keep what you and the apps create there writable by the group.
- **Config you write** (templates): a read-only bind mount.

An app runs as the deployment owner through `user:`, or through `PUID` and `PGID` for an image that starts as root and switches to them.
Only gluetun-gateway, wireguard-lan and wireguard-wan run as root, to set up their network, and they still write their files as the deployment owner.

See [the ADR](adr/volume-ownership.md) for why.

### Plugs

File dependencies are declared using plugs, a two-layer symlink system that makes external dependencies explicit:
- **Plug** (committed): `local.env -> ._.local.env` points to the socket
- **Socket** (gitignored): `._.local.env` contains or links to the actual file, and `.=.data` is or links to the actual directory
- **Sample** (committed): `.~.local.env` shows the expected structure, and its values are exp's config

Bootstrap by copying the sample: `cp .~.local.env ._.local.env`

Every env socket (`._..env` and each `._.local.env`) is treated as holding secrets: its mode is 600, and startup stops if anyone but its owner can read it.

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
