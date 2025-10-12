---- MODULE ledger ----
EXTENDS Naturals, Sequences

CONSTANTS Accounts, Currencies

VARIABLES balances

Init ==
  balances = [a \in Accounts |-> [c \in Currencies |-> 0]]

Transfer(aFrom, aTo, currency, amount) ==
  /\ amount > 0
  /\ balances[aFrom][currency] >= amount
  /\ balances' = [balances EXCEPT !(aFrom)[currency] = @ - amount,
                                 !(aTo)[currency] = @ + amount]

Next ==
  \E aFrom, aTo \in Accounts, currency \in Currencies, amount \in Nat :
    Transfer(aFrom, aTo, currency, amount)

Inv == \A a \in Accounts : balances[a][currency] >= 0

====
