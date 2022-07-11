# Manual testing

## Prerequisites

- `tmux`
- `memcoin` (from the root: `go install ./cli/node/memcoin`)
- `crypto` (from the root: `go install ./cli/crypto`)

## Create a private key

Create a new private key (can be re-used in successive tests).

```sh
./testcreatekey.sh
```

## Start the environment

Setup the manual test environment in a tmux multi-pane window

```sh
./teststart.sh
```

Use <kbd>CTRL</kbd> + <kbd>b</kbd> and arrows to move around panes. Also
<kbd>CTRL</kbd> + <kbd>b</kbd> + <kbd>[</kbd> to scroll a window.

## Add transactions

Following commands are given as examples:

- Store a key/value on the blockchain via node 1 using default nonce -1

```sh
./teststore.sh key1 value1 1 -1
```

- List the values stored on the blockchain via node 3 using nonce 7

```sh
./testlist.sh 3 7
```

## Stop the environment

Once finished with testing, it is highly recommended to cleanly shutdown this
test setup and remove the temporary files:

```sh
./teststop.sh
```
