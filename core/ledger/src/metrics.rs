use once_cell::sync::Lazy;
use prometheus::{
    proto::MetricFamily, Encoder, IntCounterVec, IntGauge, Opts, Registry, TextEncoder,
};

static REGISTRY: Lazy<Registry> = Lazy::new(Registry::new);

pub static TRANSFERS_TOTAL: Lazy<IntCounterVec> = Lazy::new(|| {
    let counter = IntCounterVec::new(
        Opts::new(
            "ledgerd_transfers_total",
            "Number of transfers applied by ledgerd",
        ),
        &["currency"],
    )
    .expect("create transfers counter");
    REGISTRY
        .register(Box::new(counter.clone()))
        .expect("register transfers counter");
    counter
});

pub static ACCOUNT_GAUGE: Lazy<IntGauge> = Lazy::new(|| {
    let gauge = IntGauge::with_opts(Opts::new(
        "ledgerd_accounts",
        "Current number of ledger accounts managed by ledgerd",
    ))
    .expect("create accounts gauge");
    REGISTRY
        .register(Box::new(gauge.clone()))
        .expect("register accounts gauge");
    gauge
});

pub fn set_account_count(count: usize) {
    ACCOUNT_GAUGE.set(count as i64);
}

pub fn inc_transfers(currency: &str) {
    TRANSFERS_TOTAL.with_label_values(&[currency]).inc();
}

pub fn gather() -> Vec<MetricFamily> {
    REGISTRY.gather()
}

pub fn encode() -> Vec<u8> {
    let metric_families = gather();
    let mut buffer = Vec::new();
    TextEncoder::new()
        .encode(&metric_families, &mut buffer)
        .expect("encode metrics");
    buffer
}
