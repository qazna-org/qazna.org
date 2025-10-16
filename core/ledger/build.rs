fn main() -> Result<(), Box<dyn std::error::Error>> {
    let protoc = protoc_bin_vendored::protoc_bin_path()?;
    std::env::set_var("PROTOC", protoc);
    tonic_build::configure()
        .build_server(true)
        .build_client(false)
        .compile(
            &["../../api/proto/qazna/v1/ledger.proto"],
            &["../../api/proto"],
        )?;
    Ok(())
}
