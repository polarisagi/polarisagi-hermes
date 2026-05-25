ALTER TABLE sys_nodes ADD COLUMN concurrency INTEGER DEFAULT 0 CHECK (concurrency >= 0 AND concurrency <= 1000);
UPDATE sys_nodes SET concurrency = 1 WHERE provider = 'google';
