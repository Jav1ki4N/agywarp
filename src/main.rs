pub mod checker;
pub mod clash;
pub mod cli;
pub mod process;
pub mod tui;
pub mod warp;

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    cli::run_cli().await
}
