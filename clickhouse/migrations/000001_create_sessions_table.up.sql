CREATE TABLE IF NOT EXISTS session_features
(
    session_id          String,
    protocol             LowCardinality(String),
    start_time           DateTime64(3),
    inbound_packets      UInt32,
    outbound_packets     UInt32,
    inout_ratio          Float64,
    total_packets        UInt32,
    inbound_bytes        UInt64,
    outbound_bytes       UInt64,
    total_bytes          UInt64,
    mean_inter_arrival   Float64,
    min_inter_arrival    Int64,
    max_inter_arrival    Int64,
    std_inter_arrival    Float64,
    mean_pkt_size        Float64,
    min_pkt_size         UInt64,
    max_pkt_size         UInt64,
    std_pkt_size          Float64,
    duration              Int64,
    throughput_mbps       Float64,
    inserted_at            DateTime DEFAULT now()
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(start_time)
ORDER BY (protocol, start_time, session_id)
SETTINGS index_granularity = 8192;