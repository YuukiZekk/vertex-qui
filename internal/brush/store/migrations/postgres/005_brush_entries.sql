CREATE TABLE brush_entries (
	id BIGSERIAL PRIMARY KEY,
	feed_id BIGINT NOT NULL REFERENCES brush_feeds (id) ON DELETE CASCADE,
	guid TEXT NOT NULL,
	title TEXT NOT NULL,
	size_bytes BIGINT NOT NULL,
	link TEXT NOT NULL,
	published_at BIGINT NOT NULL,
	first_seen_at BIGINT NOT NULL,
	status TEXT NOT NULL,
	reason TEXT NOT NULL DEFAULT '',
	info_hash TEXT NOT NULL DEFAULT '',
	instance_id INTEGER NOT NULL DEFAULT 0,
	decided_at BIGINT NOT NULL DEFAULT 0,
	UNIQUE (feed_id, guid)
)
