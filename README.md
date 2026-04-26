# mh-gobot

> ⚠️ **This project is currently in active development**

A Discord bot that bridges Discord and game servers via RCON. Listens to live game events (kills, score, chat, match state) and posts them to Discord channels, with player stat persistence.

## Features

- Live game event streaming over RCON (killfeed, scorefeed, chat, matchstate)
- Player stat tracking (kills, deaths, assists, score) via SQLite
- Auto-recovery on RCON connection loss
- Persistent leaderboard and server status embeds in Discord (image-based)
- RCON connection pooling for ad-hoc commands
- Player vs player kill ledger with historical tracking
- Skirmish score tracker: round/match win bonuses with team size and score margin factors
- Discord slash commands: `/score`, `/place`, `/top`, `/versus`, `/nemesis`, `/prey`, `/rconx`
- In-game chat commands: `!score`, `!roll`, `!versus`

## Requirements

- Go 1.25+
- Docker (optional)
- A game server with RCON support

## Configuration

Copy `.env.example` to `.env` and fill in the values:

| Variable | Required | Description |
|---|---|---|
| `DC_TOKEN` | yes | Discord bot token |
| `RCON_ADDRESS` | yes | Game server RCON host |
| `RCON_PORT` | yes | Game server RCON port |
| `RCON_PASSWORD` | yes | Game server RCON password |
| `POP_CHANNEL` | no | Channel ID for the server population embed |
| `EVENTS_CHANNEL` | no | Channel ID for live game event messages |
| `LEADERBOARDS_CHANNEL` | no | Channel ID for the persistent leaderboard embed |
| `GAME_CMD_PREFIX` | no | In-game command prefix (default: `!`) |
| `SKIRMISH_WIN_CAP` | no | Round wins needed to win a skirmish match (default: `10`) |

## Usage

```sh
# Run locally
make run

# Build only
make build

# Docker
make docker-build
make docker-run

# Docker detached
make docker-run-detached
make docker-kill-detached
```

## Scoring

In **deathmatch**, score mirrors the raw game score directly.

In **skirmish**, score is an overall performance rating built from two sources:

- **Raw game score** — points awarded by the game naturally for kills, assists, objectives, etc.
- **Skirmish bonuses** — extra points awarded on top at the end of rounds and matches.

**Round bonus** — awarded to the winning team at the end of every round, proportional to individual contribution that round. Amplified if the winning team was outnumbered.

**Match win bonus** — awarded to the winning team at match end, based on performance across the *entire* match, not just the last round. A dominant victory is worth more than a close one. Being outnumbered amplifies it further.

**Consolation bonus** — the losing team receives a small bonus at match end proportional to their overall match contribution.

Players who contributed nothing in a round or match receive no bonus for it.

## Data

Player stats are stored in `~/.mh-gobot/data.db` (SQLite). The database is created automatically on first run.

Optional map art for the alternative skirmish pop embed (`SKIRMISH_ALT_POP_TYPE=1`) is loaded from `~/.mh-gobot/imgmap/{map}.{ext}`, where `{map}` matches the RCON map ID (e.g. `miniband`, `slope`, `yard`) and `{ext}` is any image extension (`webp`, `png`, `jpg`, `gif`). If no matching file is present, the embed renders without an image. Seed art is available in `.resources/imgmap/` in this repo.

Optional default avatar for the top-20 leaderboard image (used when a player's mordhau-scribe avatar is unavailable) is loaded from `~/.mh-gobot/img/avatar_default.png`. If the file is missing, the avatar slot for that player renders empty.
