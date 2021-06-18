# Collective Signing Practical Byzantine Fault Tolerance

This section describes the implementation of an ordering service based on PBFT,
and using collective signatures for the implementation [1].

## Chain

At the very beginning of the chain, a genesis block defines the basic settings
like the roster and the authorizations (i.e. to update the roster). The special
block is not counted as a block and is formatted differently, therefore the
first block will have an index of zero.

The chain is not exactly an ordered list of blocks, but an ordered list of
**links**. Each link has a reference to the block it is pointing at, and it can
be reduced to be sent over a communication channel with the minimal piece of
information to prove its correctness.

## Leader

The consensus will assign a leader each round that will be responsible for
orchestrating the protocols. As of now, it is always the first participant that
is assigned the role of leader but it should be changed to a more random and
dynamic approach to reduce the chance of transactions being ignored.

There are plenty of research papers that offer solutions for this problem, but a
quick change could be to use the previous block signature to create a seed and
shuffle the list of participants.

## PBFT State Machine

The service is designed so that the network messages and the handlers are
implemented independently from the actual logic of PBFT. The subpackage _pbft_
contains the definition and the implementation of the state machine which has
five states:
- **None** (N): default state, only at the very beginning
- **Initial** (I): indicate the beginning of a round
- **Prepare** (P): indicate a candidate has been accepted
- **Commit** (C): indicate a threshold of participants have accepted the candidate
- **ViewChange** (V): indicate the round has expired and leader is probably
  faulty

The following schema shows the transitions allowed by the state machine:

```
PBFT State Machine

     ┌────────────── 3 ──────────────┐
     │                               │
     v                               │
   ┌───┐              ┌───┐        ┌─┴─┐
┌─>│ I ├───── 4 ─────>│ P │── 2 ──>│ C │<──┐
│  └─┬─┘              └─┬─┘        └─┬─┘   │
│    │                ^ │            │     │
│    │    ┌───┐       │ 5            │     │
│    │    │ N ├── 1 ──┘ │            │     │
│    │    └─┬─┘         v            │     │
│    │      │      ┌─────┐           │     │
│    │      └─ 5 ─>│     │           │     │
│    │             │  V  │<─── 5 ────┘     │
│    └───── 5 ────>│     │                 │
│                  └─┬─┬─┘                 │
│                    │ │                   │
└───────── 6 ────────┘ └──────── 6 ────────┘
```

1. The state machine is instantiated with an empty state (_None_) and waits for
   a candidate to transition to _Prepare_.
2. A _Prepare_ state will transition to the _Commit_ state if a valid signature
   is provided.
3. After a round is finalized, the state machine gets back to _Initial_.
4. the _Initial_ state waits for a new candidate to transition to the _Prepare_
   state.
5. Any of the state can transition to _ViewChange_ (other than (V) itself) when
   the round expires.
6. After a view change is accepted, it either transition to _Initial_ if no
   candidate has been comitted, otherwise to _Commit_.

The point (6.) is explained because we may be in a _ViewChange_ state while a
threshold of participants accept a new block. Furthermore, reaching a threshold
is a prerequisite to a candidate being committed on any participant. As such, if
a candidate has been committed on at least one of the participants, we know that
the signature exists. The system will have to finalize this candidate, even if a
new leader tries a different one. A different candidate will be refused anyway
by the participants committed to the other.

## Papers

[1] Enhancing Bitcoin Security and Performance with Strong Consistency via
Collective Signing (2016)
