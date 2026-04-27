# PRD: Mage-OS Installer

## Introduction

`mage-os-install` is a cross-platform CLI tool that sets up a fully working Mage-OS development environment in a single command. It detects whether the user has DDEV or Warden installed, guides them through an interactive terminal wizard, and produces a running Mage-OS store with Redis, OpenSearch, RabbitMQ, and Varnish pre-configured. Optional features include sample data installation and the Hyvä theme. A self-update command keeps the binary current without requiring manual downloads.

The tool targets developers who want to avoid the tedious setup ceremony of a Magento-based project and be productive in minutes.

---

## Goals

- Detect the available development environment (DDEV or Warden) automatically.
- Guide the user through all required configuration via an interactive terminal UI.
- Produce a fully working Mage-OS store with Redis, OpenSearch, RabbitMQ, and Varnish configured out of the box.
- Support optional sample data and Hyvä theme installation.
- Allow the user to review and understand the `setup:install` command before it runs.
- Resume installation from the failed step without starting over.
- Keep the binary up-to-date via a `--self-update` flag.
- Ship multiplatform binaries (Linux, macOS, Windows — amd64 and arm64) via GoReleaser.
- Verify every release with end-to-end CI tests on both DDEV and Warden.

---

## User Stories

### US-001: Project name and directory input
**Status:** done
**Priority:** 1
**Description:** As a developer, I want to specify the project name and install directory so the installer knows where to create the project.

**Acceptance Criteria:**
- [x] Installer prompts for a project name, defaulting to the current working directory's basename.
- [x] Installer prompts for an install directory, defaulting to the current working directory if the name was not changed, or `<cwd>/<name>` if a custom name was entered.
- [x] User can accept defaults by pressing Enter.
- [x] Both inputs are editable text fields.

---

### US-002: Automatic environment detection
**Status:** done
**Priority:** 1
**Description:** As a developer, I want the installer to detect which development environment I have installed so I don't have to configure it manually.

**Acceptance Criteria:**
- [x] Installer checks for `ddev` and `warden` binaries on the system PATH.
- [x] Detected environments show their name and version.
- [x] If only one environment is detected, it is selected automatically without prompting.
- [x] If both are detected, a selection screen lets the user choose.
- [x] If neither is detected, a clear error message is shown with links to install DDEV or Warden.
- [x] Detection runs asynchronously with a spinner while in progress.

---

### US-003: Admin credentials configuration
**Status:** done
**Priority:** 2
**Description:** As a developer, I want to enter admin credentials during setup so that the Mage-OS admin panel is accessible after installation.

**Acceptance Criteria:**
- [x] Form includes fields: admin username, password, email, first name, last name.
- [x] Tab and Shift+Tab navigate between fields.
- [x] All fields have sensible defaults pre-filled.
- [x] Fields are validated before proceeding.

---

### US-004: Optional sample data
**Status:** done
**Priority:** 2
**Description:** As a developer, I want to optionally install Mage-OS sample data so I have realistic content to work with.

**Acceptance Criteria:**
- [x] Setup form includes a toggle for "Install sample data" (default: off).
- [x] Space bar toggles the option.
- [x] When enabled, sample data is deployed as part of the installation steps.
- [x] CI matrix tests both with and without sample data.

---

### US-005: Optional Hyvä theme
**Status:** done
**Priority:** 2
**Description:** As a developer, I want to optionally install the Hyvä theme, providing my private repo credentials, so I can start developing with Hyvä immediately.

**Acceptance Criteria:**
- [x] Setup form includes a toggle for "Install Hyvä theme" (default: off).
- [x] When enabled, two additional fields appear: Hyvä repo URL and auth token.
- [x] Hyvä is installed and activated as part of the installation steps.
- [x] CI skips Hyvä matrix variants when the `HYVA_REPO_URL` secret is not configured.

---

### US-006: Setup:install command preview
**Status:** done
**Priority:** 3
**Description:** As a developer, I want to review the full `bin/magento setup:install` command before it runs so I understand what is being configured and can verify the flags.

**Acceptance Criteria:**
- [x] A preview screen shows the complete `setup:install` command with all flags.
- [x] Editable/user-supplied values are visually highlighted (gold color).
- [x] Long command is scrollable with up/down arrow keys.
- [x] User presses Enter to proceed or Backspace to go back and adjust settings.

---

### US-007: Multi-step installation with live logging
**Status:** done
**Priority:** 3
**Description:** As a developer, I want to see installation progress in real time so I know what is happening and can spot errors quickly.

**Acceptance Criteria:**
- [x] Installation is broken into named steps (e.g., "Configure DDEV", "Install addons", "Run setup:install").
- [x] Each step shows a status indicator: pending (•), running (▸), done (✓), failed (✗).
- [x] A scrolling log box streams output from running commands.
- [x] Completed and failed steps remain visible while the next step runs.

---

### US-008: Resume from failed step
**Status:** done
**Priority:** 3
**Description:** As a developer, I want to resume a failed installation from the step that failed so I don't have to restart the entire process after fixing an issue.

**Acceptance Criteria:**
- [x] On failure, the error screen shows the last 10 log lines.
- [x] Pressing `r` retries installation starting from the failed step.
- [x] Already-completed steps are skipped on retry.
- [x] Pressing Enter or `q` exits the installer.

---

### US-009: Open browser after successful install
**Status:** done
**Priority:** 4
**Description:** As a developer, I want the option to open the new store in my browser immediately after installation so I can verify it is working.

**Acceptance Criteria:**
- [x] After successful installation, a prompt asks the user if they want to open the store URL.
- [x] Pressing `y` opens the URL in the system default browser (macOS, Linux, Windows).
- [x] Pressing `n` or `q` exits without opening the browser.
- [x] The store URL matches the configured base URL for the selected environment.

---

### US-010: DDEV environment setup
**Status:** done
**Priority:** 1
**Description:** As a developer using DDEV, I want the installer to fully configure a DDEV project with all required services so I have a production-like stack locally.

**Acceptance Criteria:**
- [x] DDEV project is configured with correct project name, docroot, type, database, and composer settings.
- [x] OpenSearch, Redis, Cron, and RabbitMQ addons are installed.
- [x] `ddev start` is run and succeeds.
- [x] Mage-OS is installed via `composer create-project` from `repo.mage-os.org`.
- [x] `setup:install` runs inside the DDEV container with correct flags for database, cache (Redis), session (Redis), search (OpenSearch), RabbitMQ, and admin credentials.
- [x] The store is accessible at `https://<project-name>.ddev.site/` with HTTP 200.
- [x] Response includes `x-dist: Mage-OS` header.

---

### US-011: Warden environment setup
**Status:** done
**Priority:** 1
**Description:** As a developer using Warden, I want the installer to fully configure a Warden environment so I have a production-like stack locally.

**Acceptance Criteria:**
- [x] Warden environment is initialized with `warden env-init`.
- [x] SSL certificates are signed for both `<project>.test` and `app.<project>.test`.
- [x] Warden services are started.
- [x] Installer waits for PHP-FPM to be ready before running composer.
- [x] `auth.json` is copied into the container's composer home with correct ownership.
- [x] `setup:install` runs in the `php-fpm` container with correct flags.
- [x] Application settings (base URL, caching, search) are configured via `bin/magento config:set` after install.
- [x] The store is accessible at `https://app.<project>.test/` with HTTP 200.
- [x] Response includes `x-dist: Mage-OS` header.

---

### US-012: Multiplatform binary releases
**Status:** done
**Priority:** 2
**Description:** As a developer on any platform, I want to download a pre-built binary from GitHub Releases so I don't need Go installed.

**Acceptance Criteria:**
- [x] GoReleaser builds binaries for: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64, windows/arm64.
- [x] Binaries are released as plain files (not archives).
- [x] A `checksums.txt` is published alongside each release.
- [x] Release is triggered only when a `v*` tag is pushed.
- [x] Release job requires all test jobs to pass first.

---

### US-013: End-to-end CI tests
**Status:** done
**Priority:** 2
**Description:** As a maintainer, I want automated end-to-end tests on every push so regressions are caught before release.

**Acceptance Criteria:**
- [x] CI runs unit tests (`go test -v ./...`) on every push.
- [x] CI runs a full DDEV install test in a matrix of 4 variants: plain, sample data, Hyvä, Hyvä + sample data.
- [x] CI runs a full Warden install test in the same 4-variant matrix.
- [x] Each test verifies: binary runs, store is reachable at expected URL with HTTP 200, `x-dist` header is present.
- [x] Sample data variants verify product count > 0 via database query.
- [x] Hyvä variants verify correct theme ID in the database.
- [x] Failed CI runs output debugging information (docker ps, container logs, .env files).

---

### US-014: Self-update
**Status:** done
**Priority:** 3
**Description:** As a developer, I want to run `mage-os-install --self-update` to get the latest version of the binary so I don't have to manually download updates.

**Acceptance Criteria:**
- [x] Running `mage-os-install --self-update` checks the GitHub Releases API for `mage-os/mage-os-install`.
- [x] If the latest release tag is newer than the currently running binary's version, the correct binary for the current OS and architecture is downloaded.
- [x] The downloaded binary replaces the currently running executable in-place.
- [x] After replacing, the new binary is re-executed automatically (same arguments).
- [x] If the binary is already at the latest version, a message is printed ("Already up to date: vX.Y.Z") and the installer exits.
- [x] If the GitHub API is unreachable, a clear error is shown and the existing binary is left unchanged.
- [x] The downloaded binary is verified against `checksums.txt` before replacing the existing binary.
- [x] The self-update flag is handled before the TUI starts (no interactive UI required).

---

## Functional Requirements

- **FR-1:** The binary must run on Linux, macOS, and Windows for both amd64 and arm64 architectures.
- **FR-2:** The TUI must detect DDEV and Warden by checking for their binaries on the system PATH and reading their version output.
- **FR-3:** If only one environment is detected, it must be selected automatically without user interaction.
- **FR-4:** If both environments are detected, a keyboard-navigable selection screen must be shown.
- **FR-5:** If no environment is detected, installation must not proceed; a helpful error with install links must be shown.
- **FR-6:** The admin configuration form must collect: username, password, email, first name, last name, with pre-filled defaults.
- **FR-7:** The setup preview screen must highlight user-supplied values in a distinct color and support keyboard scrolling.
- **FR-8:** Installation steps must stream live output to a scrollable log box.
- **FR-9:** On failure, the installer must display the last 10 log lines and offer an `r` key to retry from the failed step.
- **FR-10:** DDEV installations must configure OpenSearch, Redis (cache + session), RabbitMQ, and Varnish via DDEV addons and `setup:install` flags.
- **FR-11:** Warden installations must configure the same services via `bin/magento config:set` after `setup:install`.
- **FR-12:** When sample data is enabled, `bin/magento sampledata:deploy` and `bin/magento setup:upgrade` must be run as installation steps.
- **FR-13:** When Hyvä is enabled, the user must provide a repo URL and auth token; the theme must be installed via composer and activated via `bin/magento setup:upgrade` and `bin/magento theme:enable`.
- **FR-14:** `mage-os-install --self-update` must query the GitHub Releases API, download the matching binary, verify it against `checksums.txt`, replace the current executable in-place, and re-execute.
- **FR-15:** Self-update must handle the case where the binary is already at the latest version gracefully (print message, exit 0).
- **FR-16:** CI must test all four install variants (plain, sample data, Hyvä, Hyvä + sample data) for both DDEV and Warden on every push.
- **FR-17:** CI release job must only run on `v*` tags and only after all test jobs pass.

---

## Non-Goals

- No support for environments other than DDEV and Warden (e.g., Docker Compose, Lando, plain localhost).
- No support for Magento Open Source or Adobe Commerce — Mage-OS only.
- No web UI or GUI; terminal only.
- No automatic update checks on every run (only on explicit `--self-update`).
- No rollback of a failed Mage-OS installation (only retry from the failed step).
- No management of an existing installation (this tool creates new environments only).
- No multi-project management or listing of installed projects.
- No Windows container support (Docker Desktop with Linux containers is assumed on Windows).

---

## Design Considerations

- **Color palette:** Orange (#F37B20) for titles and selection, Magenta (#D94F8B) for the banner, Green (#73D216) for success, Red (#EF2929) for errors, Gold (#FFD700) for editable values in the preview screen.
- **ASCII banner:** Mage-OS logo displayed at the top of the TUI.
- **Step indicators:** `•` pending, `▸` running, `✓` done, `✗` failed — consistent across all install phases.
- **Keyboard conventions:** Tab/Shift+Tab for field navigation, Space for toggles, Enter to confirm, Backspace to go back, `r` to retry, `q` to quit.
- **Sudo caching:** DDEV modifies `/etc/hosts` and may require sudo; the installer prompts for the sudo password before starting installation to avoid blocking mid-install.

---

## Technical Considerations

- **Language & framework:** Go 1.24, Bubble Tea (TUI framework), Lipgloss (styling), Bubbles (spinner, text input).
- **Module path:** `github.com/mage-os/mage-os-install`.
- **Release tooling:** GoReleaser — builds plain binaries (no archives), generates `checksums.txt`.
- **Self-update implementation:** Use the GitHub REST API (`/repos/mage-os/mage-os-install/releases/latest`) to fetch the latest tag. Map current `runtime.GOOS` and `runtime.GOARCH` to the correct asset filename. Download to a temp file, verify SHA256 against `checksums.txt`, then use `os.Rename` (or a copy + rename on Windows where in-place rename of a running binary is restricted) to replace the current executable. Re-execute via `syscall.Exec` (Unix) or `os/exec` (Windows).
- **Version embedding:** The current binary version must be embedded at build time (e.g., via `-ldflags "-X main.version=vX.Y.Z"`) so `--self-update` can compare it to the latest release tag.
- **CI automation:** `expect` scripts simulate interactive TUI input in GitHub Actions; each step has a 3600s timeout.
- **Detector interface:** Adding a new environment type requires implementing the `Detector` interface (`Info`, `Steps`, `PrepareSteps`, `Detect`, `Install`, `SetupInstallFlags`, `SetupCommandPrefix`, `BaseURL`).

---

## Success Metrics

- A developer with DDEV or Warden installed can run `mage-os-install` and reach a working store URL within 10 minutes on a warm Docker cache.
- All four CI matrix variants (plain, sample data, Hyvä, Hyvä + sample data) pass for both DDEV and Warden on every push.
- `mage-os-install --self-update` replaces the binary and re-executes within 30 seconds on a normal connection.
- Zero manual steps required between running the binary and having a working Mage-OS storefront.

---

## Open Questions

- Should `--self-update` require confirmation before replacing the binary (e.g., "Update from v1.0.0 to v1.1.0? [y/N]"), or should it proceed silently?
- Should the installed version be printed on startup or only via a `--version` flag?
- On Windows, replacing a running executable requires extra steps (copy to temp, schedule delete on reboot, or use a helper process) — what is the minimum Windows version and use case to support?
