-- Dial addresses for libp2p (transport multiaddrs, without /p2p/<id>), reported by the seller process.
ALTER TABLE seller_users ADD COLUMN listen_addrs JSONB NOT NULL DEFAULT '[]'::jsonb;
