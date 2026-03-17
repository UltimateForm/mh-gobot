# ryard

> ⚠️ **This project is currently in active development**

A Discord bot that bridges Discord and game servers via RCON. Listens to live game events (kills, score, chat, match state) and posts them to Discord channels, with player stat persistence.

## Features

- Live game event streaming over RCON (killfeed, scorefeed, chat, matchstate)
- Player stat tracking (kills, deaths, assists, score) via SQLite
- Auto-recovery on RCON connection loss
- Persistent leaderboard and server status embeds in Discord
- Slash commands: `/score`, `/place`, `/top`, `/rconx`

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

## Data

Player stats are stored in `~/.ryard/data.db` (SQLite). The database is created automatically on first run.
