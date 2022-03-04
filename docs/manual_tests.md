# Manual tests

Manual tests involve using high level shell scripts in order to setup the nodes, 
connect to them, and add a transaction to the blockchain.
It is required to install tmux before running the first command.

## Create a private key

Create a new private key (can be re-used in successive tests)

```sh
test/testcreatekey.sh
```

## Start the environment

Setup the manual test environment in a tmux multi-pane window

```sh
test/teststart.sh
```

## Add transactions

Following commands are given as examples:

- Store a key/value on the blockchain using default nonce -1

```sh
test/teststore.sh key1 value1 -1
```

- List the values stored on the blockchain via node 3 using nonce 7

```sh
test/testlist.sh 3 7
```

## Stop the environment

Once finished with testing, it is highly recommended to cleanly shutdown this test setup and remove the temporary files:

```sh
test/teststop.sh
```
