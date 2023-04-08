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

/*Stores information about the devices associated with each API key.*/
CREATE TABLE devices
(
id INTEGER UNIQUE ,
user_api_key TEXT,
name TEXT,
created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
CONSTRAINT devices_pkey PRIMARY KEY (id)
);

/*Holds the manga data for each user (API key) in JSON format.*/
CREATE TABLE manga_data
(
id INTEGER UNIQUE ,
user_api_key TEXT,
data JSON NOT NULL,
created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
CONSTRAINT manga_data_pkey PRIMARY KEY (id)
);


/*Keeps track of the last sync details and status for each device.*/
CREATE TABLE manga_sync
(
id INTEGER UNIQUE ,
user_api_key TEXT,
device_id INTEGER,
last_sync TIMESTAMP NOT NULL,
status TEXT NOT NULL DEFAULT 'unknown',
created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
CONSTRAINT manga_sync_pkey PRIMARY KEY (id)
);

ALTER TABLE devices ADD FOREIGN KEY (user_api_key) REFERENCES api_key (key) ON DELETE CASCADE  ON UPDATE CASCADE;
ALTER TABLE manga_data ADD FOREIGN KEY (user_api_key) REFERENCES api_key (key) ON DELETE CASCADE  ON UPDATE CASCADE;
ALTER TABLE manga_sync ADD FOREIGN KEY (user_api_key) REFERENCES api_key (key) ON DELETE CASCADE  ON UPDATE CASCADE;
ALTER TABLE manga_sync ADD FOREIGN KEY (device_id) REFERENCES devices (id) ON DELETE CASCADE  ON UPDATE CASCADE;
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
