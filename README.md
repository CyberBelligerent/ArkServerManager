# ASA Server Cluster Manager

A pure-Go desktop application for managing **ARK: Survival Ascended**
dedicated servers on Windows. Features Cluster setup or standalone server.

---
## Pictures
| C1 | C2 | C3 |
| --- | --- | --- |
| ![Main Window](screenshots/1.%20Main%20Window.png) | ![Install Wizard](screenshots/2.%20A%20Phase%201%20Wizard.png) | ![Install Wizard](screenshots/2.%20B%20Phase%206%20Wizard.png) |
| ![Clsuter](screenshots/3.%20Cluster%20Settings.png) | ![Scheduler](screenshots/4.%20Scheduler.png) | ![Webhooks](screenshots/5.%20Webhooks.png) |
| ![Discord](screenshots/6.%20Webhook_Discord.png) | ![Installing](screenshots/7.%20UpdatingInstalling%20Server.png) | ![Backups](screenshots/8.%20Backup%20Manager.png) |

## Known Issues
- Logs won't show historical logs of a server after run, only displaying a ready state (Looking to fix)
- RCON randomly doesn't connect (Temporary fix in commit: https://github.com/CyberBelligerent/ArkServerManager/commit/2aad5b645ffb30600db69c1d008a986cbf0ae717. Looking more into this)
- SteamCMD not fully ready when first running a server (Run Update / Install again and it'll work) (FIXED: Commit https://github.com/CyberBelligerent/ArkServerManager/commit/68974fc38aacc789487068ca2323e75665c23c9b)
- Block gameanalytics.com floods your logs with errors attempting to connect (Benign) (FIXED: https://github.com/CyberBelligerent/ArkServerManager/commit/fa66b1dd0eac66a986ffcc6abcef5a49adb85e85)

## Features

- **Cluster + server CRUD** with shared cluster directory and per-server
  overrides. You can skip the cluster entirely and run **standalone servers**
  for singleton maps
- **SteamCMD integration** — auto-detect or one-click install, then
  install/update the ASA Dedicated Server inside the app
- **Live process supervision** — start/stop/restart with status state
  machine (stopped → starting → running → stopping → crashed)
- **RCON console** — full Source RCON client commands as buttons (`saveworld`, `kickplayer`,
  `banplayer`, `broadcast`, …)
- **Player tracking** — per-server online list refreshed every 30 s,
  cluster-wide play-time aggregation, kick/ban with notes
- **Settings editor** — every ARK setting in a typed registry with
  tooltips, validation, and per-stat dino-multiplier grid
- **Custom INI lines** — append arbitrary key/value pairs to Game.ini /
  GameUserSettings.ini for mod-specific settings outside the registry
- **Backups** — zip + retention + restore for individual servers or the
  whole cluster, scheduled or manual
- **Discord webhooks** — multiple destinations, scoped to global /
  cluster / server, per-event-type subscription mask, retry queue
- **Scheduler** — oneshot + cron triggers driving 9 action types
  (start/stop/restart server or cluster, backup, RCON broadcast,
  apply preset)
- **Settings presets** — diff-style overrides ("Summer Event Rates",
  "Normal Rates") that save/apply per-cluster or per-server
- **Schedule Event wizard** — pick start + end dates + two presets,
  produces two coordinated oneshot tasks ("Summer Bash — start" /
  "Summer Bash — end") under the hood
- **ARK Smart Breeding (ASB) export** — auto-writes Multipliers.ini per
  cluster + per server on every settings change
- **First-run wizard** — language -> SteamCMD -> install dir ->
  CurseForge key -> Discord webhook -> first cluster -> first server
- **Localization** — every UI string flows through a TOML-backed bundle;
  English ships embedded, additional locales drop into `./language/`
  next to the executable
- **Activity feed** — bottom-pane log of every bus event for at-a-glance
  awareness of what's happening
- **Full uninstall** — two-stage confirmation, removes data dir,
  SteamCMD install, every cluster directory, every server install dir

---

## Quick Start (Pre-built Release)

1. Download the latest release zip from the [Releases page](https://github.com/CyberBelligerent/ArkServerManager/releases/tag/v1.1)
2. Extract anywhere — e.g. `C:\Tools\ASAManager\`. Keep
   `asamanager.exe` next to the `language/` folder so translations are
   picked up.
3. Double-click `asamanager.exe`.
4. The first-run wizard walks you through language, SteamCMD, install
   directory, optional CurseForge / Discord, and an optional first
   cluster + server.
5. Click **Install / Update** on your server to fetch the ASA Dedicated
   Server files via SteamCMD. ~30 GB free disk needed for the install.

No need to run as admin, no installation required. Everything
the app writes lives under `%APPDATA%\ASAManager\` and the install
directories you choose. The **Uninstall** button on the toolbar removes
all of it cleanly.

### What the app needs

- **Windows 10 / 11** (64-bit)
- **~30 GB free disk** for the ASA server install (more for backups)
- **Outbound HTTPS** for SteamCMD downloads + optional Discord webhooks
- **Inbound UDP/TCP** on the port triple per server (defaults
  7777/27015/27020) — open them in your firewall + router

---

## Building from Source

If you'd rather compile from source instead of downloading the zip,
you'll need a working Go toolchain and a C compiler.

| Tool             | Version | Why                                           |
|------------------|---------|-----------------------------------------------|
| Go               | 1.22+   | Compiler                                      |
| MinGW-w64 (gcc)  | 13+     | CGO for Fyne's OpenGL bindings on Windows     |

### Installing MinGW-w64

I used **WinLibs (POSIX threads, UCRT runtime)**, available via winget:

```
winget install --id BrechtSanders.WinLibs.POSIX.UCRT --accept-package-agreements --accept-source-agreements
```

Verify with `gcc --version` in a fresh command line window. If it isn't
found, add the WinLibs `bin` directory to your user `PATH` (Settings →
System → About → Advanced system settings → Environment Variables) and
reopen the shell.

### Build commands

```
# clone
git clone https://github.com/CyberBelligerent/ArkServerManager.git
cd ArkSurvivalAscendedServerManager

# build a release binary
go build -ldflags="-H=windowsgui" -o asamanager.exe ./cmd/asamanager

# run
.\asamanager.exe
```

Run tests:
```
go test ./...
```

---

## How It Works (Concepts)

### Cluster vs Server

A **cluster** is a group of servers that share a `cluster_id` and a
cluster directory (where ARK stores cross-server transfers. Survivors,
tames, items uploaded between maps). A **server** in a cluster has its
own install directory, port triple (Game/Query/RCON), and settings.

Settings cascade: cluster sets the defaults, each server can override
any subset. The "Apply to All Servers" button writes the merged
effective settings to every member's `Game.ini` and
`GameUserSettings.ini`.

You can also create **standalone servers** that don't belong to any
cluster, useful for one-off maps that don't need cross-server transfers
or shared settings. In the New Server dialog pick "(none — standalone
server)" from the Cluster dropdown. Standalone servers appear at the
root of the tree with a `(standalone)` suffix and skip the
`-clusterid=` / `-ClusterDirOverride=` launch flags. Settings, presets,
backups, and the scheduler all work the same way as on cluster-scoped
servers, just without any cluster-level inheritance.

### Presets

A **preset** is a named diff of settings overrides. Save your current
state as a preset, then apply it later (or schedule it). Two starter
presets ship with the app:

- **Solo Cluster Recommended** — boosted rates suitable for a single
  player or small group (5× taming, 2× XP, 2× harvest, faster maturation). These were
  just my personal preference
- **Normal — vanilla defaults** — every key the Solo preset touches,
  reset to its catalog default. Apply this to revert a Solo cluster
  back to vanilla without manually undoing each setting.

You can save your own presets at any time via **Settings -> Save as Preset** on either a cluster or server.

### Scheduler

The scheduler polls every 10 seconds for due tasks. Each task has:

- A **trigger**: oneshot (fire once at a date/time) or cron (5-field
  expression like `0 4 * * *` for daily at 04:00)
- An **action**: start / stop / restart (server or cluster), backup,
  RCON broadcast, apply preset
- A **missed-fire policy**: skip vs run-once if the app was off when the
  task was due

The **Schedule Event** wizard creates two coordinated oneshot tasks for
event windows like Summer Bash — one applies your "event" preset on
day X, the other applies your "normal" preset on day Y. Both restart
the cluster automatically so the new launch arg (`-ActiveEvent=Summer`)
takes effect.

### ActiveEvent

ARK's seasonal events (Summer Bash, FearEvolved, WinterWonderland, …)
are activated via the `-ActiveEvent=<name>` launch arg. Set it on the
cluster (cluster-wide default) or override per-server. The catalog at
`pkg/asa/events/` ships every known stock event. The dropdown
inheritance label shows what the cluster currently resolves to so you
know what `(inherit)` means before picking it.

### Discord webhooks

Multiple webhook destinations, each with:
- A **scope** (global, a cluster, or a server)
- An **event mask** (subscribe to specific event types or "ALL")
- A **retry queue** (Discord outage doesn't drop notifications)

The wizard's optional Discord step seeds a default global webhook;
manage them later via the **Webhooks** toolbar button.

### Localization

Every UI string flows through `internal/i18n`. The English bundle is
embedded in the binary. Translators add a TOML file to `./language/`
next to the executable; the picker in **Preferences → Language** finds
it on next launch. See `language/README.md` for the full translator
guide.

---

## Where Files Live

| Location                                  | Contents                                  |
|-------------------------------------------|-------------------------------------------|
| `%APPDATA%\ASAManager\`                   | config.toml, asamanager.db, asamanager.log |
| `%APPDATA%\ASAManager\backups\`           | Default backup destination                |
| `%APPDATA%\ASAManager\asb-multipliers\`   | ASB Multipliers.ini exports               |
| `%APPDATA%\ASAManager\logs\server-<id>.log` | Per-server stdout capture               |
| `%USERPROFILE%\ASAManager\steamcmd\`      | Default SteamCMD install                  |
| Whatever you pick in the wizard           | Each server's ARK install dir             |
| Whatever you pick on the cluster          | Cluster directory (cross-map transfers)   |
| `<exe-dir>\language\`                     | Translator-supplied locale TOML files     |

The **Uninstall** button collects all of these into a deletion plan
shown to you for confirmation; nothing leaves your machine.

---

## Phase Status

The app is feature-complete for v1. See `PLAN.md` for the full phase
roadmap. Headlines:

- **Pre-Alpha**: (COMPLETE) foundations, INI engine, SteamCMD, cluster/server
  CRUD, lifecycle + RCON, players, GUI, webhooks, backups, scheduler
- **Alpha/V1**: (COMPLETE) presets + ActiveEvent + Schedule Event wizard +
  start/stop actions + per-task stagger override
- **V1 Fixes and Additions (Next)**: drain warnings, restart coalescing, churn
  warnings, optional first-class window/recurring_window triggers
- **Scheduler Composer**: Planned chaining events and actions together to call RCON commands
- **V2 Pending** (CurseForge mod browser): skipped for v1

- **Discord Addon** (Discord bot): pending

---

## Troubleshooting

**SteamCMD exits with code 7 on Windows.** This is benign — SteamCMD's
internal cleanup hits a non-fatal hiccup after a successful self-update
and reports 7. The app recognizes this and treats it as success. The
log notes it explicitly: `[note] SteamCMD exited with code 7 — benign
on Windows; install completed.`

**Scheduled task didn't fire.** Open the Schedule tab → **Debug
Engine**. The window shows every task's `next_fire` in both your local
time and UTC, plus the engine's current "now" in both. Most "didn't
fire" reports turn out to be the user's local timezone being further
from UTC than expected. The **OpenLogs Folder** button next to it pops Explorer at
`%APPDATA%\ASAManager\` so you can grab `asamanager.log` for deeper
inspection.

**Server stuck in "starting".** ASA can take 5+ minutes to fully
initialize. The supervisor switches to "running" once both the readiness
log line appears AND RCON responds. If RCON never answers, double-check
the RCON password in the server's settings tab.

**Cluster directory empty after first run.** ARK only creates the
cluster directory on the first cross-server transfer (uploading a
survivor, dino, or item between two servers in the cluster). Until
that happens there's nothing to view. The "Open in Explorer" button
offers to create the empty folder for you.

**Translation showing `[xx-yy:some.key]` everywhere.** Your locale file
is missing those keys. Pull the latest `language/en-us.toml`, copy the
new keys into your file, and translate them. The app falls back to
English silently for missing keys, so the `[xx-yy:…]` markers only
appear when the key is missing from BOTH your file and English (rare).

---

## Contributing

### Translations

The app ships English. Adding any other language is just a TOML edit
plus a PR. See **`language/README.md`** for
the full translator guide.

Quick recipe:
1. Copy `language/en-us.toml` → `language/<your-locale>.toml`
   (e.g. `it-it.toml`, `de-de.toml`, `pt-br.toml`)
2. Translate the right-hand side of each `key = "value"` line
3. Keep `%s` / `%d` / `%q` placeholders in the same order as the
   English source
4. Open a PR adding your file

### Code

Fork, branch, PR. Run `go test ./...` before submitting. Keep diffs
focused — separate refactors from feature changes.

---

## License

MIT. See `LICENSE`.
