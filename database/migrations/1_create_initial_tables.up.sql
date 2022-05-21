CREATE TABLE users (
    id VARCHAR(100) PRIMARY KEY NOT NULL,
    name VARCHAR NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL
);

CREATE TABLE lobbies (
    id VARCHAR(50) PRIMARY KEY NOT NULL,
    user_id VARCHAR(100) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    CONSTRAINT foreign_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON UPDATE CASCADE ON DELETE CASCADE
);

CREATE TABLE mods (
    id SERIAL PRIMARY KEY,
    channel_id VARCHAR(100) NOT NULL,
    mod_id VARCHAR(100) NOT NULL,
    mod_name VARCHAR NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    CONSTRAINT uniq_channel_id_mod_id UNIQUE (channel_id, mod_id)
);

CREATE TABLE bans (
    id SERIAL PRIMARY KEY,
    channel_id VARCHAR(100) NOT NULL,
    banned_id VARCHAR(100) NOT NULL,
    banned_name VARCHAR NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    CONSTRAINT uniq_channel_id_banned_id UNIQUE (channel_id, banned_id)
);
