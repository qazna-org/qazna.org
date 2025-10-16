---- MODULE ledger ----
EXTENDS Naturals

CONSTANTS Accounts, Currencies, MaxAmount, InitialHolder, InitialTokens

VARIABLES balances

vars == << balances >>

InitialState ==
  [a \in Accounts |-> [c \in Currencies |-> IF a = InitialHolder THEN InitialTokens ELSE 0]]

Init ==
  balances = InitialState

Transfer(aFrom, aTo, currency, amount) ==
  /\ aFrom \in Accounts
  /\ aTo \in Accounts
  /\ aFrom # aTo
  /\ currency \in Currencies
  /\ amount \in 1..MaxAmount
  /\ balances[aFrom][currency] >= amount
  /\ balances' = [balances EXCEPT ![aFrom][currency] = @ - amount,
                                 ![aTo][currency] = @ + amount]

Next ==
  \E aFrom, aTo \in Accounts :
    \E currency \in Currencies :
      \E amount \in 1..MaxAmount :
        Transfer(aFrom, aTo, currency, amount)

Spec ==
  Init /\ [][Next]_vars

TypeOK ==
  balances \in [Accounts -> [Currencies -> Nat]]

NonNegativeBalances ==
  \A a \in Accounts : \A c \in Currencies : balances[a][c] >= 0

RECURSIVE SumBalances(_, _, _)
SumBalances(setAccounts, currency, bal) ==
  IF setAccounts = {} THEN 0
  ELSE
    LET account == CHOOSE a \in setAccounts : TRUE
    IN bal[account][currency]
       + SumBalances(setAccounts \ {account}, currency, bal)

Total(bal, currency) ==
  SumBalances(Accounts, currency, bal)

Conservation ==
  \A c \in Currencies : Total(balances, c) = Total(InitialState, c)

====
