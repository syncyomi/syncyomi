<h1 align="center">
  <img alt="syncyomi logo" src=".github/images/logo.png" width="160px"/><br/>
  SyncYomi
</h1>

<p align="center">SyncYomi is an open-source project designed to offer a seamless synchronization experience for your Tachiyomi manga reading progress and library across multiple devices. This server can be self-hosted, allowing you to sync your Tachiyomi library effortlessly.</p>

<p align="center"><img alt="GitHub release (latest by date)" src="https://img.shields.io/github/v/release/SyncYomi/SyncYomi?style=for-the-badge">&nbsp;<img alt="GitHub all releases" src="https://img.shields.io/github/downloads/SyncYomi/SyncYomi/total?style=for-the-badge">&nbsp;<img alt="GitHub Workflow Status" src="https://img.shields.io/github/actions/workflow/status/SyncYomi/SyncYomi/release.yml?style=for-the-badge"><img alt="Discord" src="https://img.shields.io/discord/1099009852791083058?label=Discord&logo=discord&logoColor=blue&style=for-the-badge"></p>

<!-- <img alt="syncyomi ui" src=".github/images/syncyomi-front.png"/><br/> -->


<!-- ## Documentation -->

<!-- Installation guide and documentation can be found at https://syncyomi.com -->

## Key features

- User-friendly and mobile-optimized web UI.
- Developed using Go and Vue, making SyncYomi lightweight and versatile, suitable for various platforms (Linux, FreeBSD, Windows, macOS) and architectures (e.g., x86, ARM).
- Excellent container support (Docker, k8s/Kubernetes).
- Compatible with both PostgreSQL and SQLite database engines.
- Notifications supported via Discord, Telegram, and Notifiarr.
- Base path/subfolder (and subdomain) support for easy reverse-proxy integration.

## Installation

<!-- Full installation guide and documentation can be found at https://syncyomi.com -->

Head to [releases](https://github.com/SyncYomi/SyncYomi/releases) and download the binary for your operating system. Then, run the binary.


### Docker compose

docker-compose for syncyomi. Modify accordingly if running with unRAID or setting up with Portainer.

* Logging is optional
* Host port mapping might need to be changed to not collide with other apps
* Change `BASE_DOCKER_DATA_PATH` to match your setup. Can be simply `./data`
* Set custom network if needed

Create `docker-compose.yml` and add the following. If you have a existing setup change to fit that.

```yml
version: "3.9"

services:
  syncyomi:
    container_name: syncyomi
    image: ghcr.io/syncyomi/syncyomi:latest
    restart: unless-stopped
    environment:
      - TZ=${TZ}
    user: 1000:1000
    volumes:
      - ${BASE_DOCKER_DATA_PATH}/syncyomi/config:/config
    ports:
      - 8282:8282
```

Then start with

    docker compose up -d

### Windows

<!-- Check the windows setup guide [here](https://syncyomi.com/installation/windows) -->

### Linux generic


Download the latest release, or download the [source code](https://github.com/SyncYomi/SyncYomi/releases/latest) and build it yourself using `make build`.

```bash
wget $(curl -s https://api.github.com/repos/SyncYomi/SyncYomi/releases/latest | grep download | grep linux_x86_64 | cut -d\" -f4)
```

#### Systemd (Recommended)

On Linux-based systems, it's recommended to run SyncYomi as a service with auto-restarting capabilities to ensure minimal downtime. The most common approach is to use systemd.

You will need to create a service file in `/etc/systemd/system/` called `syncyomi.service`.

```bash
touch /etc/systemd/system/syncyomi@.service
```

Then place the following content inside the file (e.g. via nano/vim/ed):

```prolog
[Unit]
Description=SyncYomi service for %i
After=syslog.target network-online.target

[Service]
Type=simple
User=%i
Group=%i
ExecStart=/usr/bin/syncyomi --config=/home/%i/.config/syncyomi/

[Install]
WantedBy=multi-user.target
```

Start the service. Enable will make it startup on reboot.

```bash
systemctl enable -q --now --user syncyomi@$USER
```

By default, the configuration is set to listen on `127.0.0.1`. It is highly recommended to use a reverse proxy like caddy, nginx or traefik.

If you are not running a reverse proxy change `host` in the `config.toml` to `0.0.0.0`.

## Community

Come join us on [Discord](https://discord.gg/aydqBWAZs8)!
