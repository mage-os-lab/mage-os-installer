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

```sh
go install github.com/mage-os-lab/mage-os-installer@latest
```

Or build from source:

```sh
git clone https://github.com/mage-os/mage-os-install.git
cd mage-os-install
go build .
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

## TODO/Whishlist

- Build pipeline.
- Toggle sampledata.
- Install Hyvä.
- Add self-update option.