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

CREATE TABLE sync_data
(
    id INTEGER PRIMARY KEY,
    user_api_key TEXT UNIQUE,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    data BLOB NOT NULL,
    data_etag TEXT NOT NULL,

    FOREIGN KEY (user_api_key) REFERENCES api_key (key) ON DELETE CASCADE
)
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
    `,
	`
	ALTER TABLE sync_lock RENAME TO _sync_lock_old;

    -- Create a new sync_lock table with the acquired_by column as non-unique
    CREATE TABLE sync_lock
    (
        id INTEGER PRIMARY KEY,
        user_api_key TEXT UNIQUE,
        acquired_by TEXT,
        last_sync TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
        status TEXT NOT NULL DEFAULT 'unknown',
        retry_count INT NOT NULL DEFAULT 0,
        acquired_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
        expires_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        FOREIGN KEY (user_api_key) REFERENCES api_key (key) ON DELETE CASCADE
    );

    -- Copy data from old table to new table
    INSERT INTO sync_lock (id, user_api_key, acquired_by, last_sync, status, retry_count, acquired_at, expires_at, created_at, updated_at)
    SELECT id, user_api_key, acquired_by, last_sync, status, retry_count, acquired_at, expires_at, created_at, updated_at FROM _sync_lock_old;

    -- Drop the old table
    DROP TABLE _sync_lock_old;
`,
	`ALTER TABLE manga_sync
	ADD COLUMN device_id TEXT NOT NULL DEFAULT '';
`,
	`
    CREATE TABLE sync_data
    (
        id INTEGER PRIMARY KEY,
        user_api_key TEXT UNIQUE,

        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

        data BLOB NOT NULL,
        data_etag TEXT NOT NULL,

        FOREIGN KEY (user_api_key) REFERENCES api_key (key) ON DELETE CASCADE
    )
`,
	`
	DROP TABLE IF EXISTS manga_data;
	DROP TABLE IF EXISTS manga_sync;
	DROP TABLE IF EXISTS sync_lock;
`,
}
