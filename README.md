# selfhost

This is a collection of interconnected Docker Compose files of services I self-host on my server.
Each Docker Compose file represents a container or a set of tightly coupled containers that are supposed to work together to provide a single service.

## Services

### Main services

- SWAG reverse proxy to handle of Let's Encrypt SSL certificates and expose locally hosted services in subdomains
- DuckDNS service to keep the domain-IP mapping up to date (especially useful if your ISP provider does not grant you a static public IP)
- Wireguard VPN to access locally hosted services from outside the network
- Wireguard VPN with Gluetun to also tunnel all external traffic through a third-party VPN

### Reverse proxy services

- Collection of media services
    - Sonarr to monitor TV series, automatically trigger download of episodes, and organize the media library
    - Jackett to provide torrent links for requested episodes
    - qBittorrent with Gluetun to download requested torrent links through a third-party VPN
    - Jellyfin to stream the downloaded media
- Vaultwarden to manage passwords and share them with others
- File Browser to import/export files on the server
- LibreSpeed to test download/upload speed
- Uptime Kuma to monitor the uptime of services

## Getting started

Each Docker Compose file might depend on one or more of the following files that you need to create yourself:

- values from the `/.env` file, created from `/.env.sample`
- environment files `/$SERVICE_NAME/*.env`, created from `/$SERVICE_NAME/*.env.sample` files
- environment files `/.environment/*.env`, created from `/.environment/*.env.sample` files
- configuration files in the Docker volumes, created from `/$SERVICE_NAME/volumes-samples/*.sample` files by following the specific instructions they contain

After creation of the files the Docker Compose file depends on, you can run it to launch the corresponding service.
In case of services behind proxy, you should also run SWAG to make them accessible to the local network.

## Environment setup

We leverage symbolic links and centralized `env` files to keep configurations consistent to the extent that is possible.

- The `/.env` file contains:
    - variables used both in a Compose files and as environment in a container
    - variables used in multiple Compose files
    - variables with personal data used in a Compose file
- The files in `/.environment/*.env` contain variables used as environment in multiple containers
- The files in `/$SERVICE_NAME/*.env` contain variables with personal data used as environment in a container

All other variables are hardcoded in the Compose files to keep it simple.

Unfortunately some consistencies are not currently enforced this way, mainly due to the fact that some images do not offer easy configuration via environment variables.

