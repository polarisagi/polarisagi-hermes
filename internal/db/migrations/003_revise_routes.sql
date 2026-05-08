-- Migration 003: Revised sys_routes for protocol-to-protocol routing with model mappings
-- New design: routes define source_protocol -> target_protocol translation with multiple model name mappings
-- The system automatically load-balances across all available nodes of the target protocol

DROP TABLE IF EXISTS sys_routes;

CREATE TABLE IF NOT EXISTS sys_routes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_protocol TEXT NOT NULL DEFAULT 'openai',
    target_protocol TEXT NOT NULL DEFAULT 'openai',
    model_mappings TEXT DEFAULT '[]',
    status INTEGER NOT NULL DEFAULT 1
);
