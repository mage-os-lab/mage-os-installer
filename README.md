# Mage-OS Installer

Get up & running with Mage-OS in minutes. If you have DDEV or Warden installed already, this tool will automatically detect and configure them for you.

## Features

- Automatically detects available development environments
- Configures services (OpenSearch, Redis, RabbitMQ, Varnish) out of the box

## Supported environments

| Environment | Site URL |
|-------------|----------|
| [DDEV](https://ddev.readthedocs.io/en/stable/) | `https://{project}.test` |
| [Warden](https://docs.warden.dev/) | `https://app.{project}.test` |

## Prerequisites

- [Docker](https://www.docker.com/) installed and running
- At least one supported environment installed (DDEV or Warden)

## Installation

Download the latest binary for your platform from the [releases page](https://github.com/mage-os-lab/mage-os-installer/releases/latest).

**macOS (Apple Silicon)**
```sh
sudo curl -sL -o /usr/local/bin/mage-os-install https://github.com/mage-os-lab/mage-os-installer/releases/latest/download/mage-os-install_$(curl -s https://api.github.com/repos/mage-os-lab/mage-os-installer/releases/latest | grep '"tag_name"' | cut -d'"' -f4 | tr -d v)_darwin_arm64
sudo chmod +x /usr/local/bin/mage-os-install
```

**macOS (Intel)**
```sh
sudo curl -sL -o /usr/local/bin/mage-os-install https://github.com/mage-os-lab/mage-os-installer/releases/latest/download/mage-os-install_$(curl -s https://api.github.com/repos/mage-os-lab/mage-os-installer/releases/latest | grep '"tag_name"' | cut -d'"' -f4 | tr -d v)_darwin_amd64
sudo chmod +x /usr/local/bin/mage-os-install
```

**Linux (x86_64)**
```sh
sudo curl -sL -o /usr/local/bin/mage-os-install https://github.com/mage-os-lab/mage-os-installer/releases/latest/download/mage-os-install_$(curl -s https://api.github.com/repos/mage-os-lab/mage-os-installer/releases/latest | grep '"tag_name"' | cut -d'"' -f4 | tr -d v)_linux_amd64
sudo chmod +x /usr/local/bin/mage-os-install
```

**Linux (ARM64)**
```sh
sudo curl -sL -o /usr/local/bin/mage-os-install https://github.com/mage-os-lab/mage-os-installer/releases/latest/download/mage-os-install_$(curl -s https://api.github.com/repos/mage-os-lab/mage-os-installer/releases/latest | grep '"tag_name"' | cut -d'"' -f4 | tr -d v)_linux_arm64
sudo chmod +x /usr/local/bin/mage-os-install
```

**Windows**

Download `mage-os-install_{version}_windows_amd64.exe` (or `windows_arm64.exe`) from the [releases page](https://github.com/mage-os-lab/mage-os-installer/releases/latest) and add the directory to your `PATH`.

**Via Go**
```sh
go install github.com/mage-os-lab/mage-os-installer@latest
```

## Usage

Run the installer from the directory where you want your project:

```sh
mage-os-install
```

The installer will walk you through:

1. **Project name** -- defaults to the current directory name
2. **Install directory** -- where to create the project
3. **Environment selection** -- pick DDEV or Warden (auto-selected if only one is available)
4. **Admin credentials** -- configure the Mage-OS admin user
5. **Command review** -- inspect the `setup:install` flags before running
6. **Installation** -- watch progress in real time

## Wishlist

- Toggle sample data.
- Install Hyvä.
- Add self-update option.