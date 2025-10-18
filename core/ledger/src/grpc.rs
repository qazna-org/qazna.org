use std::net::SocketAddr;

use chrono::Utc;
use tonic::{transport::Server, Request, Response, Status};

use crate::proto::qazna::v1::ledger_service_server::{LedgerService, LedgerServiceServer};
use crate::proto::qazna::v1::{
    Account as ProtoAccount, Balance as ProtoBalance, CreateAccountRequest, GetAccountRequest,
    GetBalanceRequest, ListTransactionsRequest, ListTransactionsResponse,
    Transaction as ProtoTransaction, TransferRequest, TransferResponse,
};
use crate::{Account, Ledger, LedgerError, Money, Transaction};
use prost_types::Timestamp;

pub async fn serve(addr: SocketAddr, ledger: Ledger) -> Result<(), tonic::transport::Error> {
    Server::builder()
        .add_service(LedgerServiceServer::new(GRpcLedger { ledger }))
        .serve(addr)
        .await
}

struct GRpcLedger {
    ledger: Ledger,
}

#[tonic::async_trait]
impl LedgerService for GRpcLedger {
    async fn create_account(
        &self,
        request: Request<CreateAccountRequest>,
    ) -> Result<Response<ProtoAccount>, Status> {
        let req = request.into_inner();
        let money = Money::new(req.currency, req.initial_amount);
        match self.ledger.create_account(money) {
            Ok(acc) => Ok(Response::new(to_proto_account(acc))),
            Err(err) => Err(map_error(err)),
        }
    }

    async fn get_account(
        &self,
        request: Request<GetAccountRequest>,
    ) -> Result<Response<ProtoAccount>, Status> {
        match self.ledger.get_account(&request.into_inner().id) {
            Ok(acc) => Ok(Response::new(to_proto_account(acc))),
            Err(err) => Err(map_error(err)),
        }
    }

    async fn get_balance(
        &self,
        request: Request<GetBalanceRequest>,
    ) -> Result<Response<ProtoBalance>, Status> {
        let req = request.into_inner();
        match self.ledger.get_balance(&req.id, &req.currency) {
            Ok(money) => Ok(Response::new(ProtoBalance {
                currency: money.currency,
                amount: money.amount,
            })),
            Err(err) => Err(map_error(err)),
        }
    }

    async fn transfer(
        &self,
        request: Request<TransferRequest>,
    ) -> Result<Response<TransferResponse>, Status> {
        let TransferRequest {
            from_id,
            to_id,
            currency,
            amount,
            idempotency_key,
        } = request.into_inner();
        let money = Money::new(currency, amount);
        let idem = if idempotency_key.is_empty() {
            None
        } else {
            Some(idempotency_key.as_str())
        };
        let res = self.ledger.transfer(&from_id, &to_id, money, idem);
        match res {
            Ok(tx) => Ok(Response::new(TransferResponse {
                transaction: Some(to_proto_transaction(tx)),
            })),
            Err(err) => Err(map_error(err)),
        }
    }

    async fn list_transactions(
        &self,
        request: Request<ListTransactionsRequest>,
    ) -> Result<Response<ListTransactionsResponse>, Status> {
        let req = request.into_inner();
        match self
            .ledger
            .list_transactions(req.limit as usize, req.after_sequence)
        {
            Ok((txs, next)) => Ok(Response::new(ListTransactionsResponse {
                items: txs.into_iter().map(to_proto_transaction).collect(),
                next_after: next.unwrap_or(0),
            })),
            Err(err) => Err(map_error(err)),
        }
    }
}

fn map_error(err: LedgerError) -> Status {
    match err {
        LedgerError::NotFound => Status::not_found("not found"),
        LedgerError::InsufficientFunds => Status::failed_precondition("insufficient funds"),
        LedgerError::InvalidAmount => Status::invalid_argument("invalid amount"),
        LedgerError::InvalidCurrency => Status::invalid_argument("invalid currency"),
        LedgerError::Storage(_) => Status::internal("ledger persistence failure"),
    }
}

fn to_proto_account(acc: Account) -> ProtoAccount {
    ProtoAccount {
        id: acc.id,
        created_at: Some(timestamp(acc.created_at)),
        balances: acc.balances.into_iter().collect(),
    }
}

fn to_proto_transaction(tx: Transaction) -> ProtoTransaction {
    ProtoTransaction {
        id: tx.id,
        created_at: Some(timestamp(tx.created_at)),
        from_account_id: tx.from_account_id,
        to_account_id: tx.to_account_id,
        currency: tx.currency,
        amount: tx.amount,
        idempotency_key: tx.idempotency_key.unwrap_or_default(),
        sequence: tx.sequence,
    }
}

fn timestamp(time: chrono::DateTime<Utc>) -> Timestamp {
    Timestamp {
        seconds: time.timestamp(),
        nanos: time.timestamp_subsec_nanos() as i32,
    }
}

pub use tonic::transport::Server as GrpcServer;
