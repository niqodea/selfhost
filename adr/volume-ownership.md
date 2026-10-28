# ADR: One Deployment Owner for All Data

**Decision:** Every app writes as the deployment owner, a dedicated system user that owns all state and content, and whose group you join.

## Context

Apps used to run as their images' default users, and only the apps that write shared content ran as the deployment owner, who was your own login user.
Reading app state then took `sudo`, the owner of each state directory depended on each image, and every app that wrote content ran as you.

## Solution

The deployment owner is a system user named after the Compose project, with a user and group ID of 900 plus the deployment's index.
Every app runs as the deployment owner through `user:`, or through `PUID` and `PGID` for an image that starts as root and switches to them.
Install creates each state directory owned by the deployment owner, since an image started as a non-root user cannot take ownership itself.
Each content directory has the setgid bit and a default ACL, so a file that you or an app creates there stays writable by the group.

Each volume is one of three kinds:
- **App state** is a plain bind mount of the app's directory in `volumes/`.
- **Shared content** is a plain bind mount of the app's directory in `content/`.
- **Config you write** is a read-only bind mount, so its owner does not matter.

## Considered Options

- **Named volumes for state**: Docker copies the image's directory and its owner into an empty named volume, so the volume loses the owner install set.
- **`userns-remap`**: root in every container becomes an unprivileged host user, but every app that writes content must set `userns_mode: host`, Gluetun needs the host's tunnel device passed in, and Docker downloads every image again.
- **A user per app**: stops an app that escapes its container from reading the other apps' data, at the cost of a host user with its own IDs for every app.
- **ID-mapped mounts or bindfs**: a container writes as its own user and the file is stored as owned by you, but Docker has no option for ID-mapped mounts, and bindfs passes every file access through a userspace program.

## Trade-offs

| Pros                                               | Cons                                                        |
|----------------------------------------------------|-------------------------------------------------------------|
| Content editable and state readable without `sudo` | An app that escapes its container reaches every app's data  |
| Apps never run as your login user                  | Install needs root to set the owner of each state directory |
| Each app's user is written in its compose file     | gluetun-gateway and the WireGuard servers still run as root |

## Conclusion

The data is yours whichever app writes it, so one owner, shared with you through its group, replaces an owner per image.
