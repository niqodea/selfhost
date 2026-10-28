# ADR: Volume Ownership by Kind of Data

**Decision:** Let Docker give app state the owner its image expects, and run as the deployment owner only the apps that write shared content.

## Context

Every app used to run as the deployment owner, through `user: ${PUID}:${PGID}` or the image's own `PUID` variable.
Each data folder then had to exist before the first start, already owned by those IDs, or Docker created it as root and the app could not write to it.
Some images, like radicale, syncthing and qbittorrent, switch to a user of their own when started as root, so the IDs an app ends up with depend on each image's entrypoint.

## Solution

Each volume is one of three kinds:
- **App state** is touched only by its app. It is a named volume backed by the app's folder in `volumes/`. When a named volume is empty, Docker copies the image's content and ownership into it on first start, so nobody prepares the folder and the app runs as its image's default user.
- **Shared content** is written by you and by apps. It is a plain bind mount of the app's folder in `content/`, owned by the deployment owner, and only the apps that write there run as `PUID` and `PGID`.
- **Config you write** is a read-only bind mount, so its owner does not matter.

Every volume name starts with its app's name, because all apps share one Compose project, where two volumes with the same name are one volume.

Compose resolves the relative `device` of a named volume only when `driver: local` is written out ([ref](https://github.com/compose-spec/compose-go/blob/main/paths/resolve.go)).

## Trade-offs

| Pros                                         | Cons                                                                             |
|----------------------------------------------|----------------------------------------------------------------------------------|
| No folder to prepare or chown                | Images that run as root by default run as root                                   |
| Each image runs as its authors tested it     | Reading app state takes `sudo`                                                   |
| The whole state still lives under `volumes/` | Docker only sets the owner of an empty folder                                    |
| `PUID` appears only where shared content is  | A non-root image needs its data path in the image, or an entrypoint that owns it |

Containers that run as root are root on the host if they escape, which `userns-remap` would prevent by mapping container root to an unprivileged host ID.

## Conclusion

Ownership follows who uses the data, so only shared content needs to know who the deployment owner is.
