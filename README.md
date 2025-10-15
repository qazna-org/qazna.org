# Qazna.org  
**International Monetary Infrastructure for a Transparent, Balanced, and Neutral Global Economy**  
**Version 1.0 — October 2025**  
© 2025 Qazna Foundation. All rights reserved.

---

## 🌍 Overview

**Qazna.org** is an international, non-profit financial institution designed to serve as a **monetary coordination layer** for sovereign central banks, state treasuries, and licensed financial institutions.

The platform acts as a **digital counterpart to the Bank for International Settlements (BIS)** —  
a **neutral infrastructure** where nations cooperate on emission, settlement, and liquidity in a transparent and mathematically balanced system.

Qazna is not a currency.  
It is the **protocol, infrastructure, and governance framework** through which digital sovereign money circulates safely, verifiably, and efficiently.

---

## 🎯 Mission

To build an open, scientifically grounded, and technologically verifiable global monetary system  
— one that maintains equilibrium instead of profit, trust instead of opacity,  
and collaboration instead of competition.

---

## 🧩 Core Principles

- **Neutrality:** no nation or institution owns Qazna.  
- **Transparency:** every operation is publicly auditable.  
- **Scientific Balance:** emission and liquidity follow formal, measurable equations.  
- **Open Governance:** all rules are defined and voted by member states.  
- **Security:** cryptographically verifiable transactions with post-quantum resilience.  
- **Sustainability:** energy- and computation-efficient infrastructure by design.

---

## ⚙️ Architecture Overview

Qazna is implemented as a **multi-language, modular system**, combining performance, auditability, and formal verification.

| Layer | Language / Tech | Description |
|-------|------------------|--------------|
| **Core Runtime (Ledger & Ordering)** | Rust | Deterministic ledger engine, cryptographic accounting, consensus, and append-only event store. |
| **Service & API Layer** | Go (gRPC + Protobuf) | Microservice orchestration, authorization, auditing, and state reconciliation. |
| **Protocol & Models** | Protobuf / TLA+ | Formal specification of invariants and transaction logic. |
| **Integration SDKs** | Go, TypeScript, Python | Developer interfaces for banks, treasuries, and researchers. |
| **Data Layer** | PostgreSQL + FoundationDB + Kafka | Distributed, fault-tolerant storage and streaming analytics. |
| **Governance & Analytics** | AI-driven | Real-time balancing and emission feedback loop. |

---

## 🏛️ Governance

Qazna Foundation is governed under a **non-profit international charter**, ensuring neutrality, transparency, and scientific rigor.

**Core Bodies:**
1. **General Assembly** — sovereign members (central banks)  
2. **Executive Board** — operational leadership  
3. **Technical Secretariat** — manages code, protocols, and releases  
4. **Compliance Council** — ensures adherence to BIS, FATF, and ISO20022  
5. **Audit Committee** — independent verification and transparency reports

📜 Governance details:  
[`/docs/legal/QAZNA_PARTICIPATION_CHARTER.md`](./docs/legal/QAZNA_PARTICIPATION_CHARTER.md)

---

## 💰 Monetary and Fee Model

Qazna replaces the notion of “profit” with **equilibrium**.  
Fees are dynamically adjusted to preserve systemic balance and sustainability.

| Participant | Fee | Purpose |
|--------------|-----|----------|
| Central Banks | 0% | Sovereign participation |
| Financial Institutions | 0.01–0.05% | Cross-border settlement maintenance |
| Corporates | 0.1–0.2% | Computational and compliance overhead |
| Retail / P2P | 0% | Accessibility for all citizens |

Mathematical model:  
[`/docs/legal/QAZNA_FEE_MODEL.md`](./docs/legal/QAZNA_FEE_MODEL.md)

---

## 📜 Licensing

Qazna’s code, specifications, and documentation are governed by a **Unified Open License Framework**:

| Component | License |
|------------|----------|
| Core Software | GNU AGPLv3 |
| SDKs & APIs | Apache 2.0 |
| Documentation & Protocols | CC BY-SA 4.0 |
| Logo & Brand | Qazna Trademark License (proprietary) |

Full details:  
[`/docs/legal/LICENSES.md`](./docs/legal/LICENSES.md)

---

## 🧰 Development Roadmap

| Phase | Timeline | Goal |
|--------|-----------|------|
| **Phase 1 (MVP)** | Q4 2025 | Core ledger, API, and governance prototype |
| **Phase 2** | Q1–Q2 2026 | Multi-node network pilot with 3 central banks |
| **Phase 3** | 2026–2027 | Full operational deployment with open audit infrastructure |
| **Phase 4** | 2028+ | AI-governed equilibrium model and formal certification framework |

---

## 🛡️ Security & Compliance

Qazna is designed for **regulatory-grade auditability**, meeting and exceeding:
- BIS Principles for Financial Market Infrastructures (PFMI)
- FATF AML/CFT Recommendations
- ISO/IEC 27001 and 20022 standards
- GDPR and sovereign data localization frameworks

Security contact: [security@qazna.org](mailto:security@qazna.org)

---

## 🧪 Local Development

- `cp .env.example .env` — populate required secrets (`QAZNA_POSTGRES_PASSWORD`, `QAZNA_GRAFANA_ADMIN_PASSWORD`, `QAZNA_AUTH_SECRET`) and optional `QAZNA_ALLOWED_ORIGINS` plus rate limit overrides. Set `QAZNA_LEDGER_GRPC_ADDR` to connect to an external Rust ledger.
- `make proto` — regenerate gRPC/Protobuf stubs (requires [`buf`](https://buf.build)); artifacts are written to `api/gen/go/api/proto/qazna/v1`.
- `make test` — runs `go vet` and `go test` with the local cache, including REST and gRPC integration tests.
- Default ports: HTTP `:8080`, gRPC `:9090` inside the container. Docker Compose maps gRPC to `localhost:19090` to avoid clashing with Prometheus on `9090`.
- UI entry points:
  - `http://localhost:8080/` — real-time global flow map.
  - `http://localhost:8080/admin/dashboard` — operational control center for administrators.
  - `http://localhost:8080/banks/dashboard` — liquidity and settlement console for national/central banks.
- Detailed dashboard runbook: [`docs/ui/dashboards.md`](docs/ui/dashboards.md)
- Quick gRPC check (uses [`grpcurl`](https://github.com/fullstorydev/grpcurl)):
  ```bash
  grpcurl -plaintext \
    -import-path api/proto \
    -proto api/proto/qazna/v1/health.proto \
    -d '{}' \
    localhost:19090 \
    qazna.v1.HealthService/Check
  ```
- Exposed gRPC services (`qazna.v1`): `InfoService/GetInfo` and `HealthService/Check`; readiness updates the Prometheus gauge `qazna_ready`.
- Exposed gRPC services (`qazna.v1`): `InfoService/GetInfo` and `HealthService/Check`; readiness updates the Prometheus gauge `qazna_ready`.

### Local perf sanity

- `make bench-local` – issues 1000 concurrent `/healthz` calls (50 in flight) using `hey` or `ab` and prints the observed requests per second.

---

## 📊 Transparency

Quarterly reports and public dashboards will be published at:

🌐 https://qazna.org/transparency

These include:
- Real-time emission data  
- Fee redistribution metrics  
- Security and uptime statistics  
- Global participation map

---

## 🕊️ Contact

📧 General inquiries: qazna.info@gmail.com  
🔒 Security reports: security@qazna.org  
🌐 Website: https://qazna.org

---

**Qazna Foundation** — *For a balanced, transparent, and sovereign global economy.*
