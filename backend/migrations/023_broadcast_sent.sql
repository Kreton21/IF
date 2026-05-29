-- Tracks which orders have received a broadcast email campaign.
-- Allows resuming interrupted broadcasts without resending to already-notified customers.
CREATE TABLE IF NOT EXISTS broadcast_sent (
    order_id   UUID        NOT NULL,
    campaign   TEXT        NOT NULL,
    sent_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (order_id, campaign)
);

CREATE INDEX IF NOT EXISTS idx_broadcast_sent_campaign ON broadcast_sent (campaign);
