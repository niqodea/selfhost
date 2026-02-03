# selfhost

A modular Docker Compose setup for self-hosting services on a personal server.

## Apps

| App                 | Description                                       |
|---------------------|---------------------------------------------------|
| **caddy**           | Reverse proxy with automatic Let's Encrypt SSL    |
| **ddns-updater**    | Dynamic DNS for servers without static IPs        |
| **gluetun-gateway** | VPN gateway for anonymous traffic routing         |
| **wireguard-lan**   | VPN server for remote access to local services    |
| **wireguard-wan**   | VPN server with anonymous exit via Gluetun        |
| **jellyfin**        | Media streaming server                            |
| **qbittorrent**     | Torrent client with anonymous exit via Gluetun    |
| **vaultwarden**     | Password manager (Bitwarden-compatible)           |
| **radicale**        | CalDAV/CardDAV server for contacts and calendars  |
| **git**             | Self-hosted Git server                            |
| **librespeed**      | Network speed test                                |

Both **wireguard-wan** and **qbittorrent** route their traffic through the same **gluetun-gateway** for anonymity.

## Getting Started

1. Clone the repo and create a deployment directory (e.g., `/opt/selfhost`)
2. Install base files: `scripts/install-base /opt/selfhost`
3. Install apps: `scripts/install-apps /opt/selfhost`
4. Copy socket samples to create actual sockets (`.~.foo` → `._.foo`)
5. Install systemd template: `scripts/install-systemd-template`
6. Enable startup service: `scripts/configure-systemd /opt/selfhost up`

## Project Structure

```
apps/
├── docker-compose.yaml               # Shared Docker resources (networks)
├── route-failsafe                    # Network failsafe routing script
├── startup                           # Boots all apps
├── .env                              # Shared config
├── .common.env                       # Shared container env vars
└── <app>/
    ├── docker-compose.yaml           # Docker resources
    ├── .env -> ../.env
    │
    ├── Dockerfile                    # Custom image build
    ├── route                         # Host-level network routing script
    ├── volumes/                      # Persistent data and configs
    ├── local.env                     # Container env vars
    └── common.env -> ../.common.env

scripts/                              # Installation and configuration scripts
adr/                                  # Architecture Decision Records
```

## Design Patterns

### Gluetun as Network Gateway

Docker can't natively use a container as a network gateway.
A few iptables rules on the host solve this, routing all traffic from the `gluetun` network through the Gluetun container.

Services opt into VPN routing by joining the network with `gw_priority: 1` and setting `dns` to the gateway address.
One Gluetun instance serves all services that need anonymous egress.

### Breadcrumbs

Breadcrumbs are a navigation pattern using specially-named symlinks (prefixed with `..`) that create a trail back to target directories.
Each breadcrumb points either further up the chain (`../..foo`) or to the current directory (`.`) when the target is reached.

```
apps/caddy/..apps  ->  ../..apps       (points up one level)
apps/..apps        ->  .               (marks arrival at apps/)
```

This approach provides more explicit and self-documenting navigation than using bare `../` in symlinks: the breadcrumb name tells you _what_ you're pointing to, not just _where_.
Learn more at the [breadcrumbs repo](https://github.com/niqodea/breadcrumbs).

### Plugs

File dependencies are declared using plugs, a two-layer symlink system that makes external dependencies explicit:
- **Plug** (committed): `local.env -> ._.local.env` points to the socket
- **Socket** (gitignored): `._.local.env` contains or links to the actual file
- **Sample** (committed): `.~.local.env` shows expected structure with placeholder values

Bootstrap by copying the sample: `cp .~.local.env ._.local.env`

This makes dependencies visible in version control without committing environment-specific paths or secrets.
Learn more at the [plugs repo](https://github.com/niqodea/plugs).

## License

MIT
