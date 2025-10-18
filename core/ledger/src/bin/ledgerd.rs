use clap::Parser;
use hyper::service::{make_service_fn, service_fn};
use hyper::{Body, Response, Server};
use std::net::SocketAddr;
use std::path::PathBuf;
use std::str::FromStr;
use tokio::signal;

use qazna_ledger::{grpc::serve, metrics, Ledger, Money};

#[derive(Parser, Debug)]
#[command(name = "ledgerd", about = "Qazna ledger runtime")]
struct Args {
    #[arg(long, default_value = "0.0.0.0:9091")]
    addr: String,
    #[arg(long, default_value = "/var/lib/ledger/state.json")]
    state_path: PathBuf,
    #[arg(long, default_value = "0.0.0.0:9102")]
    metrics_addr: String,
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args = Args::parse();
    let addr = SocketAddr::from_str(&args.addr)?;
    let metrics_addr = SocketAddr::from_str(&args.metrics_addr)?;

    let ledger = Ledger::with_persistence(&args.state_path)?;

    if ledger.is_empty() {
        // seed example account for demo runs
        let _ = ledger.create_account(Money::new("QZN", 1_000_000));
    }

    println!("ledgerd listening on {}", addr);
    println!("metrics exposed on {}", metrics_addr);

    let metrics_handle = tokio::spawn(async move {
        if let Err(err) = serve_metrics(metrics_addr).await {
            eprintln!("metrics server error: {err}");
        }
    });

    tokio::select! {
        res = serve(addr, ledger.clone()) => {
            metrics_handle.abort();
            res?;
        }
        _ = signal::ctrl_c() => {
            println!("shutting down");
            metrics_handle.abort();
        }
    }
    Ok(())
}

async fn serve_metrics(addr: SocketAddr) -> Result<(), hyper::Error> {
    let make_svc = make_service_fn(|_| async {
        Ok::<_, hyper::Error>(service_fn(|_| async {
            let body = metrics::encode();
            Ok::<_, hyper::Error>(
                Response::builder()
                    .status(200)
                    .header("Content-Type", "text/plain; version=0.0.4")
                    .body(Body::from(body))
                    .expect("metrics response"),
            )
        }))
    });

    Server::bind(&addr).serve(make_svc).await
}
