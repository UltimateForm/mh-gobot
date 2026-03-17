# ryard

> ⚠️ **This project is currently in active development**

A Discord bot that bridges Discord and game servers via RCON. Listens to live game events (kills, score, chat, match state) and posts them to Discord channels, with player stat persistence.

## Features

- Live game event streaming over RCON (killfeed, scorefeed, chat, matchstate)
- Player stat tracking (kills, deaths, assists, score) via SQLite
- Auto-recovery on RCON connection loss
- Recurring server status embed in Discord

## Requirements

- Go 1.25+
- Docker (optional)
- A game server with RCON support

## Configuration

Copy `.env.example` to `.env` and fill in the values:

```env
DC_TOKEN=
RCON_ADDRESS=
RCON_PORT=
RCON_PASSWORD=
POP_CHANNEL=
EVENTS_CHANNEL=
```

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
