DROP INDEX IF EXISTS idx_inference_matches_buyer;
DROP TABLE IF EXISTS inference_matches;

DROP INDEX IF EXISTS idx_billing_tx_party_created;
DROP TABLE IF EXISTS billing_transactions;

DROP TABLE IF EXISTS buyer_billing;
DROP TABLE IF EXISTS seller_billing;

DROP INDEX IF EXISTS idx_seller_models_seller_id;
DROP TABLE IF EXISTS seller_models;

DROP INDEX IF EXISTS seller_users_email_lower;
DROP TABLE IF EXISTS seller_users;

DROP INDEX IF EXISTS buyer_users_email_lower;
DROP TABLE IF EXISTS buyer_users;
