CREATE TABLE brush_site_daily (
	site_id BIGINT NOT NULL,
	day TEXT NOT NULL,
	uploaded BIGINT NOT NULL,
	downloaded BIGINT NOT NULL,
	PRIMARY KEY (site_id, day)
)
