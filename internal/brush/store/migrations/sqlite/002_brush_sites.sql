CREATE TABLE brush_sites (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	base_url TEXT NOT NULL,
	cookie_encrypted TEXT NOT NULL,
	created_at BIGINT NOT NULL
)
