memcoin --config /tmp/node2 minogrpc join \
    --address 127.0.0.1:2001 $(memcoin --config /tmp/node1 minogrpc token)
memcoin --config /tmp/node3 minogrpc join \
    --address 127.0.0.1:2001 $(memcoin --config /tmp/node1 minogrpc token)
memcoin --config /tmp/node1 ordering setup\
    --member $(memcoin --config /tmp/node1 ordering export)\
    --member $(memcoin --config /tmp/node2 ordering export)\
    --member $(memcoin --config /tmp/node3 ordering export)
memcoin --config /tmp/node1 access add \
    --identity $(crypto bls signer read --path private.key --format BASE64_PUBKEY)
memcoin --config /tmp/node2 access add \
    --identity $(crypto bls signer read --path private.key --format BASE64_PUBKEY)
memcoin --config /tmp/node3 access add \
    --identity $(crypto bls signer read --path private.key --format BASE64_PUBKEY)
memcoin --config /tmp/node1 pool add\
    --key private.key\
    --args go.dedis.ch/dela.ContractArg --args go.dedis.ch/dela.Access\
    --args access:grant_id --args 0300000000000000000000000000000000000000000000000000000000000000\
    --args access:grant_contract --args go.dedis.ch/dela.Evoting\
    --args access:grant_command --args all\
    --args access:identity --args $(crypto bls signer read --path private.key --format BASE64_PUBKEY)\
    --args access:command --args GRANT
memcoin --config /tmp/node1 dkg init
memcoin --config /tmp/node2 dkg init
memcoin --config /tmp/node3 dkg init
memcoin --config /tmp/node1 dkg setup --member $(memcoin --config /tmp/node1 dkg export) --member $(memcoin --config /tmp/node2 dkg export) --member $(memcoin --config /tmp/node3 dkg export)
memcoin --config /tmp/node1 shuffle init
memcoin --config /tmp/node2 shuffle init
memcoin --config /tmp/node3 shuffle init
memcoin --config /tmp/node1 e-voting initHttpServer --portNumber 2000