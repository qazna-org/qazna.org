fn main() -> Result<(), Box<dyn std::error::Error>> {
    tonic_build::configure()
        .build_server(true)
        .build_client(false)
        .out_dir("src/proto")
        .compile(
            &["../../api/proto/qazna/v1/ledger.proto"],
            &["../../api/proto"],
        )?;
    Ok(())
}
