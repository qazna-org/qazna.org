package main

import (
    "context"
    "fmt"
    "log"
    "math/rand"
    "os"
    "time"

    "qazna.org/internal/ledger"
    "qazna.org/internal/ledger/remote"
)

func main() {
    addr := os.Getenv("QAZNA_LEDGER_GRPC_ADDR")
    if addr == "" {
        addr = "localhost:9091"
    }

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    client, err := remote.Dial(ctx, addr)
    cancel()
    if err != nil {
        log.Fatalf("dial ledgerd at %s: %v", addr, err)
    }
    defer client.Close()

    svc := remote.NewService(client)
    rand.Seed(time.Now().UnixNano())

    ctxOp, cancelOp := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancelOp()

    accA, err := svc.CreateAccount(ctxOp, ledger.Money{Currency: "QZN", Amount: 1_000})
    if err != nil {
        log.Fatalf("create account A: %v", err)
    }
    accB, err := svc.CreateAccount(ctxOp, ledger.Money{Currency: "QZN", Amount: 0})
    if err != nil {
        log.Fatalf("create account B: %v", err)
    }

    transferAmt := int64(420)
    _, err = svc.Transfer(ctxOp, accA.ID, accB.ID, ledger.Money{Currency: "QZN", Amount: transferAmt}, fmt.Sprintf("smoke-%d", rand.Int()))
    if err != nil {
        log.Fatalf("transfer: %v", err)
    }

    balA, err := svc.GetBalance(ctxOp, accA.ID, "QZN")
    if err != nil {
        log.Fatalf("balance A: %v", err)
    }
    balB, err := svc.GetBalance(ctxOp, accB.ID, "QZN")
    if err != nil {
        log.Fatalf("balance B: %v", err)
    }

    if balA.Amount+balB.Amount != 1_000 {
        log.Fatalf("ledger conservation failed: %d + %d", balA.Amount, balB.Amount)
    }
    if balA.Amount != 1_000-transferAmt || balB.Amount != transferAmt {
        log.Fatalf("unexpected balances: A=%d B=%d", balA.Amount, balB.Amount)
    }

    fmt.Printf("✅ ledgerd smoke test passed: accounts=%s,%s\n", accA.ID, accB.ID)
}
