package database

import (
	"database/sql"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	_ "github.com/lib/pq"
	"github.com/scribble-rs/scribble.rs/auth"
	"github.com/scribble-rs/scribble.rs/twitch"
)

const Type = "postgres"

type Executor interface {
	Get(dest interface{}, query string, args ...interface{}) error
	Select(dest interface{}, query string, args ...interface{}) error
	Exec(query string, args ...interface{}) (sql.Result, error)
	NamedExec(query string, arg interface{}) (sql.Result, error)
	NamedQuery(query string, arg interface{}) (*sqlx.Rows, error)
}

type DB struct {
	Executor *sqlx.DB
}

type UserDigest struct {
	Id   string
	Name string
}

func FromDatabaseUrl(databaseUrl string) (*DB, error) {
	db, err := sqlx.Open(Type, databaseUrl)
	if err != nil {
		return nil, err
	}

	return &DB{Executor: db}, nil
}

func (d *DB) UpsertUser(user *auth.User) error {
	_, err := d.Executor.NamedQuery(`INSERT INTO users (id, name, created_at, updated_at) VALUES (:id, :name, NOW(), NOW()) ON CONFLICT (id) DO UPDATE SET name = :name, updated_at = NOW()`, struct {
		Id   string `db:"id"`
		Name string `db:"name"`
	}{
		Id:   user.Id,
		Name: user.Name,
	})

	return err
}

func (d *DB) AddLobby(user *auth.User, lobbyId string) error {
	_, err := d.Executor.Exec("INSERT INTO lobbies (id, user_id, created_at) VALUES ($1, $2, NOW())", lobbyId, user.Id)
	return err
}

func (d *DB) GetModsForChannel(channelId string) (*[]UserDigest, error) {
	var rows []struct {
		ModId   string `db:"mod_id"`
		ModName string `db:"mod_name"`
	}

	err := d.Executor.Select(&rows, "SELECT mod_id, mod_name FROM mods WHERE channel_id = $1", channelId)
	if err != nil {
		return nil, err
	}

	ids := make([]UserDigest, len(rows))
	for i, row := range rows {
		ids[i] = UserDigest{
			Id:   row.ModId,
			Name: row.ModName,
		}
	}

	return &ids, nil
}

func (d *DB) SetModsForChannel(channelId string, mods []twitch.ModeratorEntry) error {
	if len(mods) == 0 {
		_, err := d.Executor.Exec("DELETE FROM mods WHERE channel_id = $1", channelId)
		return err
	}

	modIds := make([]string, len(mods))
	modNames := make([]string, len(mods))

	for i, entry := range mods {
		modIds[i] = entry.UserId
		modNames[i] = entry.UserName
	}

	modIdArray := pq.Array(modIds)
	modNameArray := pq.Array(modNames)
	_, err := d.Executor.Exec("DELETE FROM mods WHERE channel_id = $1 AND mod_id NOT IN ($2)", channelId, modIdArray)
	if err != nil {
		return err
	}

	_, err = d.Executor.Exec("INSERT INTO mods (channel_id, mod_id, mod_name, created_at) SELECT $1, UNNEST($2::varchar[]), UNNEST($3::varchar[]), NOW() ON CONFLICT (channel_id, mod_id) DO NOTHING", channelId, modIdArray, modNameArray)
	if err != nil {
		return err
	}

	return nil
}

func (d *DB) GetBannedForChannel(channelId string) (*[]UserDigest, error) {
	var rows []struct {
		BannedId   string `db:"banned_id"`
		BannedName string `db:"banned_name"`
	}

	err := d.Executor.Select(&rows, "SELECT banned_id, banned_name FROM bans WHERE channel_id = $1", channelId)
	if err != nil {
		return nil, err
	}

	ids := make([]UserDigest, len(rows))
	for i, row := range rows {
		ids[i] = UserDigest{
			Id:   row.BannedId,
			Name: row.BannedName,
		}
	}

	return &ids, nil
}

func (d *DB) SetBannedForChannel(channelId string, banned []twitch.BannedUserEntry) error {
	if len(banned) == 0 {
		_, err := d.Executor.Exec("DELETE FROM bans WHERE channel_id = $1", channelId)
		return err
	}

	bannedIds := make([]string, len(banned))
	bannedNames := make([]string, len(banned))

	for i, entry := range banned {
		bannedIds[i] = entry.UserId
		bannedNames[i] = entry.UserName
	}

	bannedIdArray := pq.Array(bannedIds)
	bannedNameArray := pq.Array(bannedNames)

	_, err := d.Executor.Exec("DELETE FROM bans WHERE channel_id = $1 AND banned_id NOT IN ($2)", channelId, bannedIdArray)
	if err != nil {
		return err
	}

	_, err = d.Executor.Exec("INSERT INTO bans (channel_id, banned_id, banned_name, created_at) SELECT $1, UNNEST($2::varchar[]), UNNEST($3::varchar[]),NOW() ON CONFLICT (channel_id, banned_id) DO NOTHING", channelId, bannedIdArray, bannedNameArray)
	if err != nil {
		return err
	}

	return nil
}

func (d *DB) GetLastLobbyForUser(username string) (string, error) {
	var row struct {
		LobbyId string `db:"id"`
	}

	err := d.Executor.Get(&row, "SELECT id FROM lobbies WHERE user_id = (SELECT id FROM users WHERE name ILIKE $1 ORDER BY updated_at DESC LIMIT 1) ORDER BY created_at DESC LIMIT 1", username)
	return row.LobbyId, err
}
