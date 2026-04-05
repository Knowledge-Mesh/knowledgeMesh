-- Ollama API base URL and declared model tags (JSON), used by seller inference to call a local/remote Ollama server.
ALTER TABLE seller_users ADD COLUMN ollama_config JSONB;
