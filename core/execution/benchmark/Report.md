# Benchmark

This document is a technical report for internal purposes to keep track of our
progress. We describe two series of benchmarks that measure the efficiency of
different smart contract execution environment. The purpose of those benchmarks
is to get a comparison between a Unikernel-based solution and other common ones.

## Setups

The benchmark are run in go, using the native benchmark solution. The benchmark
reports the time by operation: ns/op.

The code used for those results uses version
`d936178eccb2264109ecd8fc59fc9cfd8dfb79e0` of
https://github.com/dedis/dela/tree/unikernel-initial.

The experiments are run in the same virtual machine running `Ubuntu 20.04.1
LTS` with 4 GB of RAM:

```bash
$ lscpu
Architecture:                    x86_64
CPU op-mode(s):                  32-bit, 64-bit
Byte Order:                      Little Endian
Address sizes:                   45 bits physical, 48 bits virtual
CPU(s):                          2
On-line CPU(s) list:             0,1
Thread(s) per core:              1
Core(s) per socket:              1
Socket(s):                       2
NUMA node(s):                    1
Vendor ID:                       GenuineIntel
CPU family:                      6
Model:                           142
Model name:                      Intel(R) Core(TM) i7-7660U CPU @ 2.50GHz
Stepping:                        9
CPU MHz:                         2496.000
```

Results show the average among 5 runs.

Bellow we describe the 5 different setups.

### Native

This setup runs the smart contract code directly from the benchmark, in go
code. This simulates a "native" smart contract.

```
Bench
┌───────────────┐
│       Smart C.│
└───────────────┘
```

### Unikernel with TCP

This setups uses a Unikernel smart contract that is accessed via a TCP
connection.

```
Bench                Unikernel
┌─────────┐       ┌────────────┐
│         ├──────►│   Smart C. │
└─────────┘       └────────────┘
```

### Local TCP server

In this setup, the smart contract is run in go via a TCP connection.

```
Bench               go TCP server
┌──────────┐       ┌──────────┐
│          ├──────►│  Smart C.│
└──────────┘       └──────────┘
```

### Solidity in local

This setup uses the go-ethereum library to execute a solidity smart contract in
its VM from go.

```
Bench
┌───────────────┐
│               │
│         sol VM│
│     ┌─────────┤
│     │ Smart C.│
└─────┴─────────┘
 ```

### Solidity via TCP

This setup uses the go-ethereum library to execute a solidity smart contract via
a TCP server.

```
                    go TCP server
                   ┌────────────┐
Bench              │      Sol VM│
┌──────────┐       │  ┌─────────┤
│          ├──────►│  │ Smart C.│
└──────────┘       └──┴─────────┘
```

## Experiments

We perform two series of experiment:

1. **Increment**: in this series of experiment the smart contract increments
   the input it receives by 1.
2. **Simple crypto**: in this series of experiment the smart contract performs a
   simple operation on ed25519 elliptic curves. The smart contract takes a scalar (s) in argument. It then performs `s*G`, where `G` is the ed25519 base point.

## Results

|   [ns/op]    |Native|Unikernel |TCP   |Solidity|Solidity TCP|
|--------------|------|----------|------|--------|------------|
|Increment     |0.000 |0.470     |0.014 |0.001   |0.014       |
|Simple crypto |0.005 |          |      |0.237   |            |
