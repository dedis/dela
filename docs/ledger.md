# Ledger

This section describes the design of the distributed public ledger abstraction
described in the core module.

The system is built around three abstractions that offer an API to clients so
that one can propagate transactions, create and validate that it will be
accepted, and finally wait for a confirmation that it is included, or rejected,
by the public ledger.

A transaction is what triggers a change in the ledger set of values, where it is
left to the ledger implementation to define how values are stored and
identified. The transaction is created with a targeted execution environment
alongside the input parameters.

The first abstraction which allows one to propagate transactions is defined as a
pool that can work independetly from the other abstractions. In fact, you could
run a distributed system that only shares transactions.

The second is a service that accepts a transaction and can return if it will be
included in the next block, or if it will be rejected. Furthermore, it defines
the validity of the transaction so that it cannot be reused in a _replay attack_
for instance.

Finally, the third abstraction will collect the transactions from the pool and
create a chain of blocks, block after block according to a consensus algorithm.
One can register to the service to receive notifications when a new block is
created and learn about the transactions included in the block, and which of
them have been accepted.

## Transaction Pool

Each participant of a distributed ledger needs to collect a bunch of
transactions when it is trying to create a block. The pool is there to provide
that service, so that clients can send their transactions to one of the pool of
the distributed system while the ordering services wait for enough transactions
to be discovered to start a new block.

The purpose is to offer a single entry point for the client and then the system
will take care of spreading the transactions so that any reachable member will
at some point learn about it.

## Validation Service

The validation service is there to protect the system against malicious
behaviour. We already mentionned a replay attack which allows an attacker to
listen for transactions and reuse them to double spent, or similar operation.
Bitcoin is protected because of the UTXOs, while Ethereum is using a nonce that
is monotically increasing for each address/identity.

Another potential issue is the input parameters provided by the transaction
which could allow a malicious client to execute a smart contract in a incorrect
way. The validation service makes sure the result of a transaction execution
only updated what is allowed.

Finally, because it is the validation that defines what the transaction looks
like, it also provides a manager that will help to create and sign transactions.
For instance, in the case of Ethereum, a client needs to use the correct nonce
for the transaction to be accepted.

## Ordering Service

A distributed ledger backed with a blockchain evolves block after block. Each
block contains a list of transactions that will be execute sequentially or in
parrallel depending on the implementation. Each execution will produce zero, one
or several changes in the ledger values.

The ordering service is responsible for creating the blocks and in another
extent, the chain. The abstraction does not provide any property on how the
consensus is decided, but it defines the ways to read or get the values of the
ledger.

## Implementations

- [CoSiPBFT](cosipbft.md)
