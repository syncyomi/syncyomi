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

/*Holds the manga data for each user (API key) in JSON format.*/
CREATE TABLE manga_data
(
id SERIAL UNIQUE ,
user_api_key TEXT,
data JSON NOT NULL,
created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
CONSTRAINT manga_data_pkey PRIMARY KEY (id)
);


/*Keeps track of the last sync details and status for each device.*/
CREATE TABLE manga_sync
(
id SERIAL UNIQUE ,
user_api_key TEXT UNIQUE,
last_sync TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
status TEXT NOT NULL DEFAULT 'unknown',
created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
CONSTRAINT manga_sync_pkey PRIMARY KEY (id)
);

/*Keeps track of lock files when syncing.*/
CREATE TABLE sync_lock
(
id SERIAL UNIQUE ,
user_api_key TEXT UNIQUE,
acquired_by TEXT,
last_sync TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
status TEXT NOT NULL DEFAULT 'unknown',
retry_count INT NOT NULL DEFAULT 0,
acquired_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
expires_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
CONSTRAINT sync_lock_pkey PRIMARY KEY (id)
);

ALTER TABLE manga_data ADD FOREIGN KEY (user_api_key) REFERENCES api_key (key) ON DELETE CASCADE  ON UPDATE CASCADE;
ALTER TABLE manga_sync ADD FOREIGN KEY (user_api_key) REFERENCES api_key (key) ON DELETE CASCADE  ON UPDATE CASCADE;
ALTER TABLE sync_lock ADD FOREIGN KEY (user_api_key) REFERENCES api_key (key) ON DELETE CASCADE  ON UPDATE CASCADE;
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
}
