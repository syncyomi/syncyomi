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

- Logging is optional
- Host port mapping might need to be changed to not collide with other apps
- Change `BASE_DOCKER_DATA_PATH` to match your setup. Can be simply `./data`
- Set custom network if needed
- You may need to update the host address to 0.0.0.0 if you are running with podman

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

Download the latest release and run it.

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

## Usage

### Configuring and Running the Service

### Initial Setup

After the first run of the SyncYomi service, several files will be generated in your specified running directory. These are essential for the service's operation.

#### Configuration for Reverse Proxy Users

If you're using a reverse proxy and your setup includes a sub-directory, it's crucial to update the baseUrl value in your configuration. Additionally, adjust your proxy settings to exclude this suffix. Below is an example configuration for nginx:

```nginx
location /syncyomi/ {
    proxy_pass http://localhost:8282/;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection $http_connection;
}
```

After updating the configuration file, restart the SyncYomi service to apply these changes.

### API Key Generation

To generate an API key, access the web interface of SyncYomi at `http://<your-server-address>:8282`.
Once the service is running, navigate to `Settings > API` Keys and create a new API key. This key is crucial for linking your Tachiyomi clients to the SyncYomi service.

Important Note: Treat each API key as a unique user. To ensure a seamless syncing experience across multiple devices, it's important to use the same API key for all the devices you intend to synchronize. Using different API keys will result in the devices being treated as separate users, each with their own syncing data. Keep your API key secure and consistent across all your devices for optimal functionality.

## Install The App

#### Preparing for Installation

Before proceeding, backup your existing Tachiyomi environment. ~~The latest release of the modified Tachiyomi app can be found on our [Discord](https://discord.gg/aydqBWAZs8).~~ It's merged to TachiyomiSY [TachiyomiSY Preview version](https://github.com/jobobby04/TachiyomiSYPreview/releases)
Download and Install currently it's in preview version of TachiyomiSY but eventually it will be in the stable version.

Then, go to `Settings > Data and Storage` in the app. Under the Sync section, input your Host details (e.g., `http://192.168.1.202:8282` or `https://sync.mydomain.tld`) and the previously generated API Key.

#### Important Configuration for Direct IP Users

If you are using a direct IP address (e.g., http://192.168.1.202:8282) to connect to your server, ensure the config.toml file on your server has the host set to 0.0.0.0. This setting allows connections from any IP address. Be sure to use your machine's IPv4 address. For guidance on how to find your IPv4 address, refer to an [IPv4 finding guide for Windows](https://support.microsoft.com/en-us/windows/find-your-ip-address-5cf30435-114d-41a6-9c24-eed37b8e014b).

Linux:
`ip -4 addr show | grep -oP '(?<=inet\s)\d+(\.\d+){3}'`

### Synchronization Details

Currently, synchronization occurs at fixed intervals.
If you frequently switch between devices, manually initiate a sync in the `Data and Storage` settings of the Tachiyomi app on the device you were using.
Repeat this process on the next device after the first synchronization is complete to ensure your reading progress is up-to-date across all devices.

## Community

Come join us on [Discord](https://discord.gg/aydqBWAZs8)!
