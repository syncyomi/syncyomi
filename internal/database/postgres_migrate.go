package database

const postgresSchema = `
CREATE TABLE users
(
    id         SERIAL PRIMARY KEY,
    username   TEXT NOT NULL,
    password   TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (username)
);

/*Stores information about the devices associated with each API key.*/
CREATE TABLE api_key
(
    name       TEXT,
    key        TEXT PRIMARY KEY,
    scopes     TEXT []   DEFAULT '{}' NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

/*Manages notifications for various events*/
CREATE TABLE notification
(
	id         SERIAL PRIMARY KEY,
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
	id SERIAL PRIMARY KEY,
	user_api_key TEXT UNIQUE,

	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

	data BYTEA NOT NULL,
	data_etag TEXT NOT NULL,
	rendered_seq BIGINT,

	FOREIGN KEY (user_api_key) REFERENCES api_key (key) ON DELETE CASCADE
);

CREATE TABLE sync_data_history
(
	id           SERIAL PRIMARY KEY,
	user_api_key TEXT NOT NULL,
	data_etag    TEXT NOT NULL,
	size         BIGINT NOT NULL,
	created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	data         BYTEA NOT NULL,
	FOREIGN KEY (user_api_key) REFERENCES api_key (key) ON DELETE CASCADE
);
CREATE INDEX idx_sync_data_history_key ON sync_data_history (user_api_key, id DESC);

CREATE TABLE sync_device
(
	id           SERIAL PRIMARY KEY,
	user_api_key TEXT NOT NULL,
	device_id    TEXT NOT NULL,
	device_name  TEXT NOT NULL DEFAULT '',
	last_seen    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	last_event   TEXT NOT NULL DEFAULT '',
	last_status  TEXT NOT NULL DEFAULT '',
	last_message TEXT NOT NULL DEFAULT '',
	last_cursor  BIGINT NOT NULL DEFAULT 0,
	protocol     TEXT NOT NULL DEFAULT '',
	created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	UNIQUE (user_api_key, device_id),
	FOREIGN KEY (user_api_key) REFERENCES api_key (key) ON DELETE CASCADE
);

CREATE TABLE sync_status
(
	user_api_key   TEXT PRIMARY KEY,
	last_synced_at TIMESTAMP,
	last_event_at  TIMESTAMP,
	last_event     TEXT NOT NULL DEFAULT '',
	last_status    TEXT NOT NULL DEFAULT '',
	last_device    TEXT NOT NULL DEFAULT '',
	last_message   TEXT NOT NULL DEFAULT '',
	FOREIGN KEY (user_api_key) REFERENCES api_key (key) ON DELETE CASCADE
);

CREATE TABLE sync_state
(
    user_api_key TEXT PRIMARY KEY,
    seq          BIGINT NOT NULL DEFAULT 0,
    created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_api_key) REFERENCES api_key (key) ON DELETE CASCADE
);

CREATE TABLE sync_item
(
    user_api_key  TEXT NOT NULL,
    kind          TEXT NOT NULL,
    key           TEXT NOT NULL,
    parent_key    TEXT NOT NULL DEFAULT '',
    name          TEXT NOT NULL DEFAULT '',
    version       BIGINT NOT NULL DEFAULT 0,
    deleted       BOOLEAN NOT NULL DEFAULT FALSE,
    refs          TEXT NOT NULL DEFAULT '',
    payload       BYTEA NOT NULL,
    seq           BIGINT NOT NULL,
    origin_device TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (user_api_key, kind, key),
    FOREIGN KEY (user_api_key) REFERENCES api_key (key) ON DELETE CASCADE
);
CREATE INDEX idx_sync_item_seq ON sync_item (user_api_key, seq);
CREATE INDEX idx_sync_item_parent ON sync_item (user_api_key, kind, parent_key);
`

var postgresMigrations = []string{
	"",
	`
	CREATE TABLE notification
	(
		id         SERIAL PRIMARY KEY,
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
	id SERIAL UNIQUE ,
	user_api_key TEXT UNIQUE,
	acquired_by TEXT UNIQUE,
	last_sync TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	status TEXT NOT NULL DEFAULT 'unknown',
	retry_count INT NOT NULL DEFAULT 0,
	acquired_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	expires_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	CONSTRAINT sync_lock_pkey PRIMARY KEY (id)
	);
`,
	`
	ALTER TABLE sync_lock ADD FOREIGN KEY (user_api_key) REFERENCES api_key (key) ON DELETE CASCADE  ON UPDATE CASCADE;
`,
	`
    UPDATE sync_lock SET status = 'success' WHERE status = 'pending';
`,
	`
    ALTER TABLE sync_lock DROP CONSTRAINT IF EXISTS sync_lock_acquired_by_key;
`,
	`	DO $$
BEGIN
    -- Check if the column exists
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_schema = 'public'  -- or your specific schema if not default
        AND table_name = 'manga_sync'
        AND column_name = 'device_id'
    ) THEN
        -- Add the column if it does not exist
        ALTER TABLE manga_sync ADD COLUMN device_id TEXT NOT NULL DEFAULT '';
    END IF;
END $$;

`,
	`
	CREATE TABLE sync_data
	(
		id SERIAL PRIMARY KEY,
		user_api_key TEXT UNIQUE,

		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

		data BYTEA NOT NULL,
		data_etag TEXT NOT NULL,

		FOREIGN KEY (user_api_key) REFERENCES api_key (key) ON DELETE CASCADE
	)
`,
	`
	DROP TABLE IF EXISTS manga_data;
	DROP TABLE IF EXISTS manga_sync;
	DROP TABLE IF EXISTS sync_lock;
`,
	`
	CREATE TABLE sync_data_history
	(
		id           SERIAL PRIMARY KEY,
		user_api_key TEXT NOT NULL,
		data_etag    TEXT NOT NULL,
		size         BIGINT NOT NULL,
		created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		data         BYTEA NOT NULL,
		FOREIGN KEY (user_api_key) REFERENCES api_key (key) ON DELETE CASCADE
	);
	CREATE INDEX idx_sync_data_history_key ON sync_data_history (user_api_key, id DESC);

	CREATE TABLE sync_device
	(
		id           SERIAL PRIMARY KEY,
		user_api_key TEXT NOT NULL,
		device_id    TEXT NOT NULL,
		device_name  TEXT NOT NULL DEFAULT '',
		last_seen    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		last_event   TEXT NOT NULL DEFAULT '',
		last_status  TEXT NOT NULL DEFAULT '',
		last_message TEXT NOT NULL DEFAULT '',
		created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		UNIQUE (user_api_key, device_id),
		FOREIGN KEY (user_api_key) REFERENCES api_key (key) ON DELETE CASCADE
	);

	CREATE TABLE sync_status
	(
		user_api_key   TEXT PRIMARY KEY,
		last_synced_at TIMESTAMP,
		last_event_at  TIMESTAMP,
		last_event     TEXT NOT NULL DEFAULT '',
		last_status    TEXT NOT NULL DEFAULT '',
		last_device    TEXT NOT NULL DEFAULT '',
		last_message   TEXT NOT NULL DEFAULT '',
		FOREIGN KEY (user_api_key) REFERENCES api_key (key) ON DELETE CASCADE
	);
`,
	`
CREATE TABLE sync_state
(
    user_api_key TEXT PRIMARY KEY,
    seq          BIGINT NOT NULL DEFAULT 0,
    created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_api_key) REFERENCES api_key (key) ON DELETE CASCADE
);

CREATE TABLE sync_item
(
    user_api_key  TEXT NOT NULL,
    kind          TEXT NOT NULL,
    key           TEXT NOT NULL,
    parent_key    TEXT NOT NULL DEFAULT '',
    name          TEXT NOT NULL DEFAULT '',
    version       BIGINT NOT NULL DEFAULT 0,
    deleted       BOOLEAN NOT NULL DEFAULT FALSE,
    refs          TEXT NOT NULL DEFAULT '',
    payload       BYTEA NOT NULL,
    seq           BIGINT NOT NULL,
    origin_device TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (user_api_key, kind, key),
    FOREIGN KEY (user_api_key) REFERENCES api_key (key) ON DELETE CASCADE
);
CREATE INDEX idx_sync_item_seq ON sync_item (user_api_key, seq);
CREATE INDEX idx_sync_item_parent ON sync_item (user_api_key, kind, parent_key);

ALTER TABLE sync_device ADD COLUMN last_cursor BIGINT NOT NULL DEFAULT 0;
ALTER TABLE sync_device ADD COLUMN protocol TEXT NOT NULL DEFAULT '';
ALTER TABLE sync_data ADD COLUMN rendered_seq BIGINT;
`,
}
