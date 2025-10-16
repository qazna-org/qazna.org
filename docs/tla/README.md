# TLA+ Specifications

This directory will hold formal specifications for critical Qazna invariants.

## Roadmap
- `ledger.tla` — double-entry invariants for account/transfer operations.
- Integrate `tlc` checks into CI (`make tla`).

## Getting Started
1. Install [TLA+](https://lamport.azurewebsites.net/tla/tla.html) or the VSCode extension.
2. Run `make tla` to execute the TLC model checker from this repository.
