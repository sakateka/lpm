## Longest Prefix Match (LPM) in Go

This repository provides a high-performance Longest Prefix Match (LPM) data structure implemented in Go.

### Origin and Attribution

The original idea and baseline implementation are based on the `lpm.h` from the `yanet2` project. See the original source here:

- `yanet2/common/lpm.h`: https://github.com/yanet-platform/yanet2/blob/main/common/lpm.h

This project is a Go port of that implementation.

### What’s different from the original

- The original implementation required inserting prefixes sorted by length (longest to shortest). This requirement has been lifted in this Go port; you can insert prefixes in any order.
- Implemented in pure Go, with idiomatic APIs and tests.

### Repository layout

- `lpm.go`: Core LPM implementation
- `lpm_test.go` and related `*_test.go`: Test suites and benchmarks
- `examples/simple`: Minimal runnable example

### Getting started

Install dependencies and run tests:

```bash
go test ./...
```

Run the simple example:

```bash
cd examples/simple
go run .
```

### Benchmarks

Run all benchmarks with memory stats:

```bash
go test -bench=. -benchmem ./...
```

Run specific large-scale benchmarks (e.g., the 100k dataset):

```bash
go test -run=^$ -bench=100k -benchmem
```

### Shared memory

This implementation supports zero-copy shared memory usage for read-heavy, multi-process scenarios:

- Build a trie normally, then serialize it with `PackToSharedStorage()`.
- Map the resulting byte slice in other processes and load it with `NewWithSharedStorage(storage)`.
- `Stats()` reports block/value counts and approximate storage footprint across shared and dynamic data.

Notes:
- Values are limited to 255 bytes (length-prefixed), enforced during packing.
- Shared storage is ideal for read-mostly workloads; new prefixes can still be inserted dynamically after loading.
- See tests around shared storage behavior and persistence.

Run only shared-memory related tests:

```bash
go test -run SharedStorage
```

### License

This project is distributed under the terms of the license found in `LICENSE`. Please also refer to the original `yanet2` project license for their code.


