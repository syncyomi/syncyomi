package database

const sqliteSchema = `
CREATE TABLE users
(
    id         INTEGER PRIMARY KEY,
    username   TEXT NOT NULL,
    password   TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (username)
);

CREATE TABLE api_key
(
    name       TEXT,
    key        TEXT PRIMARY KEY,
    scopes     TEXT []   DEFAULT '{}' NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE notification
(
    id         INTEGER PRIMARY KEY,
    name       TEXT,
    type       TEXT,
    enabled    BOOLEAN,
    events     TEXT []   DEFAULT '{}' NOT NULL,
    token      TEXT,
    api_key    TEXT,
    webhook    TEXT,
    title      TEXT,
    icon       TEXT,
    host       TEXT,
    username   TEXT,
    password   TEXT,
    channel    TEXT,
    rooms      TEXT,
    targets    TEXT,
    devices    TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE manga_data
(
    id INTEGER PRIMARY KEY,
    user_api_key TEXT,
    data TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_api_key) REFERENCES api_key (key) ON DELETE CASCADE
);

CREATE TABLE manga_sync
(
    id INTEGER PRIMARY KEY,
    user_api_key TEXT UNIQUE,
    last_sync TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status TEXT NOT NULL DEFAULT 'unknown',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_api_key) REFERENCES api_key (key) ON DELETE CASCADE,
);
`

var sqliteMigrations = []string{
	"",
	`
	CREATE TABLE notification
	(
		id         INTEGER PRIMARY KEY,
		name       TEXT,
		type       TEXT,
		enabled    BOOLEAN,
		events     TEXT []   DEFAULT '{}' NOT NULL,
		token      TEXT,
		api_key    TEXT,
		webhook    TEXT,
		title      TEXT,
		icon       TEXT,
		host       TEXT,
		username   TEXT,
		password   TEXT,
		channel    TEXT,
		rooms      TEXT,
		targets    TEXT,
		devices    TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	`,
}
