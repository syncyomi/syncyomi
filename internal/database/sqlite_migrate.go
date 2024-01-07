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
    FOREIGN KEY (user_api_key) REFERENCES api_key (key) ON DELETE CASCADE
);

CREATE TABLE sync_lock
(
    id INTEGER PRIMARY KEY,
    user_api_key TEXT UNIQUE,
    acquired_by TEXT UNIQUE,
    last_sync TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status TEXT NOT NULL DEFAULT 'unknown',
    retry_count INT NOT NULL DEFAULT 0,
    acquired_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_api_key) REFERENCES api_key (key) ON DELETE CASCADE
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
	`
	CREATE TABLE sync_lock
	(
		id INTEGER PRIMARY KEY,
		user_api_key TEXT UNIQUE,
		acquired_by TEXT UNIQUE,
		last_sync TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		status TEXT NOT NULL DEFAULT 'unknown',
		retry_count INT NOT NULL DEFAULT 0,
		acquired_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		expires_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_api_key) REFERENCES api_key (key) ON DELETE CASCADE
	);
`,
	`
    -- Migration to change default status to 'success' in sync_lock table
    PRAGMA foreign_keys=off;
    BEGIN TRANSACTION;
    ALTER TABLE sync_lock RENAME TO _sync_lock_old;
    CREATE TABLE sync_lock
    (
        id INTEGER PRIMARY KEY,
        user_api_key TEXT UNIQUE,
        acquired_by TEXT UNIQUE,
        last_sync TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
        status TEXT NOT NULL DEFAULT 'success',  -- Changed to 'success'
        retry_count INT NOT NULL DEFAULT 0,
        acquired_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
        expires_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        FOREIGN KEY (user_api_key) REFERENCES api_key (key) ON DELETE CASCADE
    );
    INSERT INTO sync_lock (id, user_api_key, acquired_by, last_sync, status, retry_count, acquired_at, expires_at, created_at, updated_at)
        SELECT id, user_api_key, acquired_by, last_sync, 'success', retry_count, acquired_at, expires_at, created_at, updated_at FROM _sync_lock_old;
    DROP TABLE _sync_lock_old;
    COMMIT;
    PRAGMA foreign_keys=on;
    `,
}
