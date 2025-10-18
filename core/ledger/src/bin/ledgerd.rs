use clap::Parser;
use std::net::SocketAddr;
use std::path::PathBuf;
use std::str::FromStr;
use tokio::signal;

use qazna_ledger::{grpc::serve, Ledger, Money};

#[derive(Parser, Debug)]
#[command(name = "ledgerd", about = "Qazna ledger runtime")]
struct Args {
    #[arg(long, default_value = "0.0.0.0:9091")]
    addr: String,
    #[arg(long, default_value = "/var/lib/ledger/state.json")]
    state_path: PathBuf,
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args = Args::parse();
    let addr = SocketAddr::from_str(&args.addr)?;

    let ledger = Ledger::with_persistence(&args.state_path)?;

    if ledger.is_empty() {
        // seed example account for demo runs
        let _ = ledger.create_account(Money::new("QZN", 1_000_000));
    }

    println!("ledgerd listening on {}", addr);

    tokio::select! {
        res = serve(addr, ledger.clone()) => res?,
        _ = signal::ctrl_c() => {
            println!("shutting down");
        }
    }
    Ok(())
}
