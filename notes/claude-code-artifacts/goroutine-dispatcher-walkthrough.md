# `queue/goroutine.go` — line-by-line walkthrough

The file is 37 lines. We'll go top to bottom, calling out the Go-specific mechanic at each step. Companion file is `queue.go`, which defines the `Job`, `Handler`, and `Dispatcher` types this file implements — included at the end for reference.

```go
package queue
```

Every Go file starts with a `package` declaration. The package is the unit of import; everything in `api/internal/infra/queue/*.go` belongs to this one package. The directory name and the package name are usually the same (here, `queue`). Other Go files import this as `accordli.com/analyze-ai/api/internal/infra/queue` — the directory path under the module root — and refer to its exports as `queue.Job`, `queue.Dispatcher`, etc.

**Visibility rule:** identifiers starting with a capital letter are exported (public outside the package); lowercase are package-private. So `Dispatcher` is public, `mu` (a struct field below) is private.

```go
import (
    "context"
    "fmt"
    "sync"
)
```

Standard library imports. `context` is Go's request-scoped value/cancellation/deadline carrier. `fmt` is formatted I/O (we use it for `Errorf`). `sync` is concurrency primitives — mutexes, wait groups, atomic ops.

Imports are explicit and unused imports are a compile error (Go is strict about this).

```go
type GoroutineDispatcher struct {
    mu       sync.RWMutex
    handlers map[string]Handler
}
```

A struct definition — like a class without methods (methods come below, attached separately). Two fields:

- `mu` is a `sync.RWMutex`. **Read/Write mutex** — multiple readers can hold it at once, but a writer is exclusive. We use this because `Register` and `Enqueue` both touch the same `handlers` map; concurrent map access without a mutex is a runtime panic in Go.
- `handlers` is a `map[string]Handler` — Go's built-in hash map, here keyed by a string (the job kind, like `"review_run.execute"`) and valued by a `Handler` (defined in `queue.go` as `func(ctx context.Context, j Job) error` — yes, functions are first-class values you can store).

**Embedding the mutex as a struct field is the idiomatic Go pattern** for protecting per-instance state. You don't inherit; you compose. The mutex protects the *fields of this struct instance*, not anything else.

```go
func NewGoroutine() *GoroutineDispatcher {
    return &GoroutineDispatcher{handlers: map[string]Handler{}}
}
```

A constructor function. Go has no `new` keyword in the OO sense; the convention is `NewT()` returning a `*T` (pointer to a fresh `T`).

`&GoroutineDispatcher{...}` is a *composite literal* with `&` to take its address — equivalent to "allocate a new struct, fill in these fields, give me a pointer to it." Fields not mentioned (`mu`) are zero-valued. The zero value of a `sync.RWMutex` is a valid, ready-to-use unlocked mutex — that's a deliberate Go design choice; you never have to call an `init()` on the mutex.

`map[string]Handler{}` is the empty-but-non-nil map literal. **A nil map can be read but not written to** (writing panics), so we explicitly initialize it. Maps are reference types — copying the struct doesn't deep-copy the map.

Returning `*GoroutineDispatcher` (pointer) instead of `GoroutineDispatcher` (value) matters: callers will mutate the mutex and the map, which only works through a pointer.

```go
func (d *GoroutineDispatcher) Register(kind string, h Handler) {
    d.mu.Lock()
    defer d.mu.Unlock()
    d.handlers[kind] = h
}
```

A method. The `(d *GoroutineDispatcher)` between `func` and the name is the **receiver** — the equivalent of `self`/`this`, but you name it explicitly and choose its type. `*GoroutineDispatcher` (pointer receiver) lets the method mutate the struct's fields. A value receiver `(d GoroutineDispatcher)` would copy the struct on call, including the mutex, which would silently break concurrency. Convention: if any method on a type takes a pointer receiver, all of them do.

What this method does:

- `d.mu.Lock()` — acquire the *write* lock (exclusive). Other goroutines calling `Lock` or `RLock` block until we release.
- `defer d.mu.Unlock()` — `defer` schedules a call to run when the function returns, no matter how it returns (early return, panic, etc.). This is Go's RAII equivalent and is the canonical way to pair a `Lock`/`Unlock`. Without it, an early return or panic between `Lock` and `Unlock` would deadlock everything else.
- `d.handlers[kind] = h` — map assignment. Now any subsequent `Enqueue` for that kind has a handler to call.

`Register` is called at startup from `main.go`:

```go
q.Register(reviewrun.JobKind, orchestrator.Handler)
```

So we're saying "when a job of kind `review_run.execute` arrives, call this method on the orchestrator."

```go
func (d *GoroutineDispatcher) Enqueue(_ context.Context, j Job) error {
    d.mu.RLock()
    h, ok := d.handlers[j.Kind]
    d.mu.RUnlock()
```

Another method, this time the public dispatch entry point.

The first parameter is `_ context.Context`. The **blank identifier** `_` means "I don't intend to use this." We're satisfying the `Dispatcher` interface (which requires a `ctx`), but we don't actually need it inside the function body. If we named it `ctx` and didn't use it, the compiler wouldn't complain (unlike unused variables, unused parameters are fine), but `_` is a clearer signal of intent.

`d.mu.RLock()` / `RUnlock()` — *read* lock. Other readers can also hold it at the same time, but if a writer is trying to `Lock`, both block until everyone is out. We use the read lock here because we're only reading from `handlers`, not mutating it.

`h, ok := d.handlers[j.Kind]` is the **comma-ok idiom** for map lookup. Map indexing in Go returns either one or two values:

- `h := d.handlers[j.Kind]` returns the zero value of the value type if the key isn't present. For `Handler` (a function type), the zero value is `nil`. Calling a nil function panics — bad.
- `h, ok := d.handlers[j.Kind]` returns the value and a bool. `ok` is `true` if the key was present. This is the safe form for "I'm not sure if it's there."

`:=` is short-variable-declaration: it both declares and assigns in one step, with the type inferred from the right-hand side. After this line, `h` has type `Handler` and `ok` has type `bool`.

We deliberately release the read lock *before* doing anything with `h`, because the lock only needs to protect the map read. Holding it across the goroutine launch below would be unnecessary and would block `Register` calls during dispatch.

```go
    if !ok {
        return fmt.Errorf("queue: no handler registered for kind %q", j.Kind)
    }
```

Standard Go error pattern. Functions that can fail return an `error` as their last return value. `fmt.Errorf` builds an error from a format string. The `%q` verb prints a Go-quoted version of the value — for a string like `review_run.execute`, that yields `"review_run.execute"` with literal quotes. Useful in error messages because it makes whitespace and odd characters visible.

```go
    go func() {
        // New context: the enqueuing request can return before the job
        // runs. Same shape as a River-dispatched job.
        _ = h(context.Background(), j)
    }()
    return nil
}
```

This is the heart of the file. Three things happen on these four lines:

1. **`func() { ... }`** is an anonymous function literal — define a function inline, right here.
2. **`go`** prefix launches it as a *goroutine*: a separately-scheduled, lightweight thread of execution. Goroutines are cheap (a few KB of stack, multiplexed onto OS threads by the Go runtime) and starting one is essentially free.
3. **`()`** at the end *calls* the anonymous function (with no arguments). Without the parens, you'd just have a function value. With them, you're invoking it. The `go` keyword schedules that invocation in a new goroutine instead of running it synchronously.

So: "spawn a goroutine that runs `h(context.Background(), j)`, ignore its result, and don't wait for it."

The `_ = h(...)` discards the returned `error`. We can't usefully return it to the caller — `Enqueue` has already returned `nil` to its caller by the time the handler runs. The handler is responsible for its own error logging (and indeed `orchestrator.Handler` does log via `o.Log.Error`).

**Why `context.Background()` instead of the `ctx` parameter?** The HTTP request that called `Enqueue` will return its response and complete *before* the orchestrator finishes the Run. If we passed the request's `ctx`, then when the request ends, Go's context machinery cancels it — and any code in the handler that respects context cancellation would suddenly be told "stop, you're cancelled" mid-run. That's wrong. The job's lifetime is independent of the HTTP request's lifetime. `context.Background()` is "a fresh root context with no deadline and no cancellation" — a clean slate. The comment on those lines spells this out.

This is also the pattern River will use when we swap it in: River-launched workers pull a job off the queue and run it with their own root context, not the caller's.

Then `Enqueue` itself returns `nil` — meaning "I successfully enqueued the job; whether it succeeds when it runs is a different question."

---

## What this means semantically

`GoroutineDispatcher.Enqueue` is essentially:

```go
go orchestrator.Handler(context.Background(), job)
return nil
```

…with a tiny bit of indirection so the *interface* (`Dispatcher`) matches the one River will satisfy. The seam pattern works because `Enqueue` and the registered `Handler` look the same whether the underlying impl is "literally a goroutine" or "a row in a Postgres table that a River worker eventually picks up." The orchestrator code doesn't have to change.

What we lose with the goroutine impl:

- **Persistence.** No row anywhere says "this job is in flight." A `kill -9` of the process loses the work.
- **Retries.** Handler returns an error → discarded. No retry-on-failure machinery.
- **Backpressure.** Each `Enqueue` immediately spawns a goroutine. 10,000 simultaneous requests = 10,000 goroutines running pipeline work, hammering the DB and the LLM API.
- **Observability.** No queue depth, no oldest-job age, no dead-letter view.
- **Cross-process work.** Anything calling `Enqueue` must be in the same Go process as the registered handler. Splitting api ↔ worker into two containers (Phase 8) requires a real queue.

All of those are fine for Phase 0–2 SoloMocky / dev work. Phase 3 (River) is what fixes them.

---

## Concept reference

A short table of the Go mechanics that show up in this file, in case any of them are new:

| Mechanic | Where it appears | What it does |
|---|---|---|
| Capital-letter export | `Dispatcher`, `mu` | Capital = visible outside the package; lowercase = package-private |
| Pointer receiver `(d *T)` | All methods | Lets the method mutate `d`'s fields; required when the struct holds a mutex |
| `&T{...}` composite literal | `NewGoroutine` | Allocate a struct and take its address |
| Zero values | `mu` field | Uninitialized = ready-to-use for `sync.RWMutex` (and most stdlib types) |
| `defer` | `Register` | Run this call on function exit, no matter how it exits |
| `RLock`/`Lock` distinction | Both methods | Read lock = many concurrent readers; Write lock = exclusive |
| Comma-ok map lookup | `Enqueue` | `v, ok := m[k]` distinguishes "absent" from "zero value" |
| Blank identifier `_` | `_ context.Context`, `_ = h(...)` | "I'm intentionally ignoring this" |
| `go` keyword | `Enqueue` | Run this function call in a new goroutine |
| Anonymous function `func(){}()` | `Enqueue` | Define + immediately invoke a function inline |
| `context.Background()` | inside the goroutine | Fresh root context, decoupled from any request's lifetime |
| `error` as last return value | `Enqueue` | Standard Go failure signaling; nil = success |
| `fmt.Errorf` with `%q` | `Enqueue` | Build an error with a Go-quoted string interpolated |

---

## For reference: `queue.go` (the interface)

```go
package queue

import "context"

type Job struct {
    ID   string
    Kind string
    Args []byte
}

type Handler func(ctx context.Context, j Job) error

type Dispatcher interface {
    Register(kind string, h Handler)
    Enqueue(ctx context.Context, j Job) error
}
```

Things to notice:

- `Handler` is a *named function type*. Anywhere a `Handler` is expected, you can pass any function with the matching signature. `orchestrator.Handler` has signature `func(context.Context, queue.Job) error` and so it satisfies `queue.Handler`.
- `Dispatcher` is an interface — a list of method signatures. **Go interfaces are satisfied implicitly:** `*GoroutineDispatcher` satisfies `Dispatcher` because it has both `Register` and `Enqueue` with matching signatures. There's no `implements Dispatcher` keyword. This is what makes the seam pattern feel weightless to write — to add `RiverDispatcher` later, we just write a struct with the same two methods, and any code that took a `Dispatcher` will accept it.

---

./notes/claude-code-artifacts/goroutine-dispatcher-walkthrough.md
