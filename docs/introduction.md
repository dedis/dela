![Infography](assets/infograph.png)

# Modulus

Modulus is a set of modules definition and their implementations. We choose this
name for this project because it captures well the idea of providing a modular
framework that allows one to either use a module's implementation or define its
own. Fabric is not about a particular blockchain implementation, it's about the
definitions of modular pieces that build a blockchain.

## Terminologies

- **actor** - An actor is a player of a protocol or a module. It is intended to
  be accessible only after the initialization and it provides the primitives to
  start the underlying protocol logic.

- **arc** - Arc stands for Access Rights Control. It is the abstraction that
  controls the access to the instances.

- **blockchain** - A blockchain is a distributed and immutable storage
  abstraction. A well-defined threshold of participants work together to reach a
  consensus on every block.

- **cosi** - CoSi stands for *Collective Signature*. It represents an aggregate
  of signature from multiple key pairs and it can be verified by the
  corresponding aggregate of public keys.

- **footprint** - Footprint defines a digest commonly produced by a hash
  algorithm that can be used to verify the integrity of some data. One example
  is the inventory page integrity to prove which instances are stored.

- **instance** - An instance is the smallest unit of storage in a ledger. It is
  identified by a unique key and stores a generic piece of data.

- **inventory** - An inventory is the storage abstraction of a ledger. The
  ledger evolves alongside with the blocks and that is represented by pages in
  an inventory where the index matches the block index.

- **ledger** - A ledger is a book of records of transactions. Similarly, a
  public distributed ledger can be implemented on top of a blockchain.

- **message** - A message is a serialized data structure that can be transmitted
  over a physical channel and decoded on the other side.

- **mino** - Mino stands for *Minimalist Network Overlay*, it is the abstraction
  that defines how to register and use RPCs over a distributed set of nodes.

- **node** - A node is a server participating in a protocol.

- **payload** - A payload is the data that a block will store. The blockchain
  implementation does not know the data structure thus requires a
  *PayloadProcessor* that will validate during the consensus.

- **proof** - A proof is a cryptographic tool that can provide integrity to a
  piece of data.

- **protobuf** - https://developers.google.com/protocol-buffers/

- **roster** - A roster is a set of participants to a protocol.

- **RPC** - RPC stands for *Remote Procedure Call*. It represents a procedure
  that an authorized external actor can call to get a specific result.

- **skipchain** - A skipchain is a specific implementation of the blockchain
  that is using collective signings to create shortcuts between blocks.
