CREATE TABLE brush_peer_daily (
	day TEXT NOT NULL,
	site_id BIGINT NOT NULL,
	instance_id INTEGER NOT NULL,
	country_code TEXT NOT NULL,
	uploaded BIGINT NOT NULL,
	downloaded BIGINT NOT NULL,
	PRIMARY KEY (day, site_id, instance_id, country_code)
)
