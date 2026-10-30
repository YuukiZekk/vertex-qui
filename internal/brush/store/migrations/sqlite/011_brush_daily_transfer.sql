CREATE TABLE brush_daily_transfer (
	instance_id INTEGER NOT NULL,
	day TEXT NOT NULL,
	downloaded BIGINT NOT NULL,
	uploaded BIGINT NOT NULL,
	last_alltime_dl BIGINT NOT NULL,
	last_alltime_ul BIGINT NOT NULL,
	last_sample_at BIGINT NOT NULL,
	PRIMARY KEY (instance_id, day)
)
