[![progress-banner](https://backend.codecrafters.io/progress/redis/7fe55803-209b-444a-814c-9e2cca4a0fc4)](https://app.codecrafters.io/users/codecrafters-bot?r=2qF)

# Redis clone in Go

A toy Redis server written from scratch in Go, with no third-party dependencies. It speaks the real Redis wire protocol (RESP), so any Redis client, including `redis-cli`, can talk to it.

This is my solution to the CodeCrafters
["Build Your Own Redis" challenge](https://codecrafters.io/challenges/redis). My main goals are to sharpen my Go, practice writing clean and well-structured code, and work through some nice algorithmic problems (stream IDs, blocking queues, expiry). Learning how Redis works under the hood is a welcome bonus.

## Supported commands for now

| Area | Commands | Notes |
| --- | --- | --- |
| Connection | `PING`, `ECHO` | |
| Strings | `SET`, `GET` | `SET` supports the `PX` option (expiry in milliseconds). Expired keys are removed lazily on `GET`. |
| Lists | `RPUSH`, `LPUSH`, `LRANGE`, `LLEN`, `LPOP` | `LPOP` supports the optional count. `LRANGE` supports negative indexes. |
| Blocking | `BLPOP` | Blocks until an element is pushed or the timeout expires. A timeout of `0` waits forever. |
| Streams | `TYPE`, `XADD` | `XADD` supports explicit IDs, `<ms>-*` and `*` (fully auto-generated), and rejects IDs that aren't strictly increasing. |


## How it works

```
app/
├── main.go      TCP listener, one goroutine per connection, command dispatch
├── command.go   Handlers for strings and lists (SET, GET, RPUSH, BLPOP, ...)
├── stream.go    Streams: TYPE, XADD and stream ID logic
└── resp/        RESP parser and encoders (no dependency on the server)
```

- **Protocol.** Clients send commands as RESP arrays of bulk strings. The `resp` package parses them into a small `RESP` value and provides encoders for simple strings, errors, integers, bulk strings and arrays.
- **Concurrency.** Every accepted connection is served by its own goroutine, and all of them share one in-memory `Server` holding the keyspace (`map[string]*Entry`). Each `Entry` stores a value of any type (string, list, stream) plus an optional expiry time.
- **Blocking commands.** `BLPOP` registers a channel in a per-key queue of waiters. A later `RPUSH` on that key hands its first element directly to the longest-waiting client, which keeps the wake-up order fair. On timeout, the client removes itself from the queue, and the code handles the race where a push arrives just as the timer fires.

## Running it

You need Go 1.25 or later.

```sh
./your_program.sh
```

The server listens on port `6379`. In another terminal:

```sh
redis-cli PING                          # PONG
redis-cli SET greeting hello PX 5000    # OK, expires in 5 seconds
redis-cli GET greeting                  # "hello"
redis-cli RPUSH queue a b c             # (integer) 3
redis-cli BLPOP jobs 0                  # blocks until another client pushes to "jobs"
redis-cli XADD events '*' temp 21       # auto-generated stream ID
```

`redis-cli` is optional. Any RESP client works, and so does `nc` if you type the protocol by hand.
