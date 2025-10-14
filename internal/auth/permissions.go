package auth

const (
	PermLedgerCreateAccount = "ledger.account.create"
	PermLedgerTransfer      = "ledger.transfer"
)

var BuiltinPermissions = []Permission{
	{Key: PermLedgerCreateAccount, Description: "Create ledger accounts"},
	{Key: PermLedgerTransfer, Description: "Transfer funds between accounts"},
}
