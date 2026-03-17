package data

import "errors"

var DbPlayerUpsertError error = errors.New("failed to upsert player")
var DbPlayerNotFound error = errors.New("player not found")
var DbPlayerReadError error = errors.New("failed to read player")
var DbFailedToCommitDbTransaction error = errors.New("failed to commit db transaction")
var DbInvalidPlayer error = errors.New("invalid player")
var DbMetaNotFound error = errors.New("meta key not found")
