CREATE TABLE brush_feeds (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	site_id INTEGER NOT NULL REFERENCES brush_sites (id) ON DELETE CASCADE,
	name TEXT NOT NULL,
	url_encrypted TEXT NOT NULL,
	interval_seconds INTEGER NOT NULL,
	min_size_bytes BIGINT NOT NULL,
	max_size_bytes BIGINT NOT NULL,
	free_only INTEGER NOT NULL,
	skip_hr INTEGER NOT NULL,
	window_seconds INTEGER NOT NULL,
	instance_id INTEGER NOT NULL,
	enabled INTEGER NOT NULL,
	etag TEXT NOT NULL DEFAULT '',
	last_modified TEXT NOT NULL DEFAULT '',
	last_fetch_at BIGINT NOT NULL DEFAULT 0,
	last_error TEXT NOT NULL DEFAULT '',
	created_at BIGINT NOT NULL
)
