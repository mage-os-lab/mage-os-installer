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
curl -sL https://github.com/mage-os-lab/mage-os-installer/releases/latest/download/mage-os-installer_$(curl -s https://api.github.com/repos/mage-os-lab/mage-os-installer/releases/latest | grep '"tag_name"' | cut -d'"' -f4 | tr -d v)_darwin_arm64.tar.gz | tar -xz mage-os-install
sudo mv mage-os-install /usr/local/bin/
```

**macOS (Intel)**
```sh
curl -sL https://github.com/mage-os-lab/mage-os-installer/releases/latest/download/mage-os-installer_$(curl -s https://api.github.com/repos/mage-os-lab/mage-os-installer/releases/latest | grep '"tag_name"' | cut -d'"' -f4 | tr -d v)_darwin_amd64.tar.gz | tar -xz mage-os-install
sudo mv mage-os-install /usr/local/bin/
```

**Linux (x86_64)**
```sh
curl -sL https://github.com/mage-os-lab/mage-os-installer/releases/latest/download/mage-os-installer_$(curl -s https://api.github.com/repos/mage-os-lab/mage-os-installer/releases/latest | grep '"tag_name"' | cut -d'"' -f4 | tr -d v)_linux_amd64.tar.gz | tar -xz mage-os-install
sudo mv mage-os-install /usr/local/bin/
```

**Linux (ARM64)**
```sh
curl -sL https://github.com/mage-os-lab/mage-os-installer/releases/latest/download/mage-os-installer_$(curl -s https://api.github.com/repos/mage-os-lab/mage-os-installer/releases/latest | grep '"tag_name"' | cut -d'"' -f4 | tr -d v)_linux_arm64.tar.gz | tar -xz mage-os-install
sudo mv mage-os-install /usr/local/bin/
```

**Windows**

Download `mage-os-installer_{version}_windows_amd64.zip` (or `windows_arm64.zip`) from the [releases page](https://github.com/mage-os-lab/mage-os-installer/releases/latest), extract it, and add the directory to your `PATH`.

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