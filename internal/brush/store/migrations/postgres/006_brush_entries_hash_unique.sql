CREATE UNIQUE INDEX brush_entries_hash_instance ON brush_entries (info_hash, instance_id) WHERE status IN ('adding', 'added')
