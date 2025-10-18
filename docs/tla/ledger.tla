---- MODULE ledger ----
EXTENDS Naturals, Sequences

CONSTANT MaxAmount

Accounts == {"NB_KZ", "NB_UZ", "NB_AZ"}
Currencies == {"QZN", "UZS", "AZN"}

InitialAmount(a, c) ==
  CASE a = "NB_KZ" /\ c = "QZN" -> 1000
    [] a = "NB_UZ" /\ c = "UZS" -> 800
    [] a = "NB_AZ" /\ c = "AZN" -> 600
    [] OTHER -> 0

InitialBalances == [
  a \in Accounts |-> [c \in Currencies |-> InitialAmount(a, c)]
]

LedgerOrder == <<
  <<"NB_KZ", "NB_UZ", "QZN", 200>>,
  <<"NB_UZ", "NB_AZ", "UZS", 150>>,
  <<"NB_AZ", "NB_KZ", "AZN", 100>>
>>

VARIABLES balances, sequence

vars == << balances, sequence >>

Init ==
  /\ balances = InitialBalances
  /\ sequence = << >>

CanTransfer(bal, aFrom, currency, amount) ==
  bal[aFrom][currency] >= amount

ApplyTransfer(bal, aFrom, aTo, currency, amount) ==
  [bal EXCEPT
    ![aFrom][currency] = @ - amount,
    ![aTo][currency] = @ + amount
  ]

TransferStep(aFrom, aTo, currency, amount) ==
  /\ aFrom \in Accounts
  /\ aTo \in Accounts
  /\ aFrom # aTo
  /\ currency \in Currencies
  /\ amount \in 1..MaxAmount
  /\ CanTransfer(balances, aFrom, currency, amount)
  /\ balances' = ApplyTransfer(balances, aFrom, aTo, currency, amount)
  /\ sequence' = Append(sequence, <<aFrom, aTo, currency, amount>>)

Next ==
  LET n == Len(sequence) IN
    IF n < Len(LedgerOrder)
      THEN LET step == LedgerOrder[n + 1] IN
             TransferStep(step[1], step[2], step[3], step[4])
      ELSE /\ balances' = balances /\ sequence' = sequence

Spec ==
  Init /\ [][Next]_vars

TypeOK ==
  /\ balances \in [Accounts -> [Currencies -> Nat]]
  /\ sequence \in Seq(Accounts \X Accounts \X Currencies \X Nat)

NonNegativeBalances ==
  \A a \in Accounts : \A c \in Currencies : balances[a][c] >= 0

RECURSIVE SumBalances(_,_)
RECURSIVE SumInitial(_,_)
SumBalances(accSet, currency) ==
  IF accSet = {} THEN 0
  ELSE LET acct == CHOOSE a \in accSet : TRUE
       IN balances[acct][currency] + SumBalances(accSet \ {acct}, currency)

SumInitial(accSet, currency) ==
  IF accSet = {} THEN 0
  ELSE LET acct == CHOOSE a \in accSet : TRUE
       IN InitialBalances[acct][currency] + SumInitial(accSet \ {acct}, currency)

Conservation ==
  \A c \in Currencies : SumBalances(Accounts, c) = SumInitial(Accounts, c)

LedgerOrderPreserved ==
  LET n == Len(sequence) IN
    /\ n <= Len(LedgerOrder)
    /\ sequence = SubSeq(LedgerOrder, 1, n)

====
