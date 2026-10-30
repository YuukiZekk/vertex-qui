CREATE TABLE brush_sites (
	id BIGSERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	base_url TEXT NOT NULL,
	cookie_encrypted TEXT NOT NULL,
	created_at BIGINT NOT NULL
)
