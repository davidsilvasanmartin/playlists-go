# Deep Dive — `withRetry` and Go Context

This document unpacks `withRetry` from `internal/musicbrainz/client_impl.go` in full detail. It covers Go-specific concepts — functions as values, integer range loops, channels, `select`, and `context` — that are likely unfamiliar if you are coming from another language. Each concept is introduced only when the code actually needs it.

```go
func withRetry(ctx context.Context, maxAttempts int, delay time.Duration, fn func() error) error {
    var err error
    for i := range maxAttempts {
        err = fn()
        if err == nil {
            return nil
        }
        if i < maxAttempts-1 {
            select {
            case <-time.After(delay):
            case <-ctx.Done():
                return ctx.Err()
            }
        }
    }
    return err
}
```

---

## 1. The function signature

```go
func withRetry(ctx context.Context, maxAttempts int, delay time.Duration, fn func() error) error
```

Four parameters. Three are straightforward; the fourth introduces an important Go concept.

**`ctx context.Context`** — a cancellation signal. Explained in full in section 3.

**`maxAttempts int`** — how many total attempts to make (not how many retries). If `maxAttempts` is 3, the function will try at most 3 times.

**`delay time.Duration`** — how long to wait between attempts. `time.Duration` is just an integer type counting nanoseconds, but Go provides constants to write it readably: `2 * time.Second`, `500 * time.Millisecond`.

**`fn func() error`** — the operation to retry, passed as a value.

### Functions as values

In Go, functions are first-class values. You can assign them to variables, pass them as arguments, and return them from other functions — exactly like an integer or a string.

```go
// A plain function
func double(n int) int {
    return n * 2
}

// Assign it to a variable
f := double
fmt.Println(f(5)) // 10

// Pass it to another function
func apply(n int, fn func(int) int) int {
    return fn(n)
}
apply(5, double) // 10
```

`fn func() error` is the type of any function that takes no arguments and returns an error. The caller wraps the operation they want retried in a function literal (an anonymous function) and passes it in:

```go
err := withRetry(ctx, 3, 2*time.Second, func() error {
    return callSomeAPI()
})
```

The `func() error { ... }` is defined inline. `withRetry` calls it up to 3 times, knowing nothing about what is inside it. This is the key design: the retry logic and the operation are completely decoupled.

---

### `for i := range maxAttempts`

Before Go 1.22 you would write:

```go
for i := 0; i < maxAttempts; i++ { ... }
```

Since Go 1.22 you can range over an integer directly:

```go
for i := range maxAttempts { ... }
```

This is exactly equivalent: `i` goes 0, 1, 2, …, `maxAttempts-1`. It is shorter and eliminates the off-by-one risk. The project's `go.mod` requires Go 1.22+, so this syntax is safe to use.

---

## 2. Channels — just enough

Before explaining `select` and `context.Done()`, you need to know what a channel is.

A **channel** is a typed pipe through which goroutines (Go's lightweight threads) send and receive values. The `<-` operator is used for both directions:

```go
ch := make(chan int)   // create a channel that carries ints

go func() {
    ch <- 42           // send 42 into the channel (blocks until someone receives)
}()

v := <-ch              // receive from the channel (blocks until a value arrives)
fmt.Println(v)         // 42
```

The critical property: **receiving from a channel blocks until a value is available.** This makes channels useful as synchronization tools, not just data pipes.

Two special cases matter for `withRetry`:

**A closed channel:** When a channel is closed, any receive on it returns immediately with the zero value. This is how `ctx.Done()` works — it returns a channel that is closed when the context is cancelled.

**A nil channel:** Receiving from a `nil` channel blocks forever and never returns. This is relevant when a context is never cancelled (e.g., `context.Background()`). `ctx.Done()` returns `nil` in that case, and the `select` statement handles it correctly — a `nil` case is simply never selected.

---

## 3. Go context — the full picture

`context.Context` is one of Go's most important concepts. It solves a problem that all server-side code faces: **how do you tell an in-progress operation to stop?**

### The problem

Imagine a user sends a search request. Your handler calls `withRetry`, which calls MusicBrainz. While the first attempt is waiting for a response, the user closes their browser. From the server's perspective, the original HTTP request is gone — the response will go nowhere. But without some cancellation mechanism, the code keeps running: it retries, waits, retries again, potentially for several seconds, wasting resources on work whose result nobody will ever use.

`context.Context` is the standard Go mechanism for propagating a cancellation signal down through a call chain — from the HTTP handler into the service, into the MusicBrainz client, into `withRetry`, into the individual HTTP calls.

### The context tree

Contexts form a tree. You always start from a root and derive children:

```
context.Background()          ← root, never cancelled
    └── WithTimeout(5s)       ← cancelled after 5 seconds
            └── WithCancel()  ← cancelled manually OR when parent times out
```

When a parent is cancelled, **all its children are cancelled automatically**. The child never outlives the parent.

The four ways to create a context:

```go
// Root contexts (starting points — use one of these at the top)
ctx := context.Background()   // never cancelled, used in main() and tests
ctx := context.TODO()         // placeholder when you haven't decided yet

// Derived contexts (always created from an existing context)
ctx, cancel := context.WithCancel(parent)              // cancelled by calling cancel()
ctx, cancel := context.WithTimeout(parent, 5*time.Second) // cancelled after 5s
ctx, cancel := context.WithDeadline(parent, time.Now().Add(5*time.Second)) // same, explicit time
```

`WithCancel`, `WithTimeout`, and `WithDeadline` all return a `cancel` function. **Always call it**, typically with `defer`:

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel() // releases resources even if the timeout fires first
```

Forgetting `cancel()` leaks memory — the context's internal timer and goroutine are never freed.

### `ctx.Done()` and `ctx.Err()`

```go
ctx.Done()  // returns a channel (<-chan struct{}) that is closed when the context is cancelled
ctx.Err()   // returns the reason: context.Canceled or context.DeadlineExceeded (nil if not cancelled)
```

`struct{}` is Go's zero-size type — a channel of `struct{}` carries no data; it is used purely as a signal. When the context is cancelled, the channel is closed, and any receive on a closed channel returns immediately.

```go
select {
case <-ctx.Done():
    fmt.Println(ctx.Err()) // context.Canceled or context.DeadlineExceeded
}
```

### The golden rule

**Pass `ctx` as the first argument to every function that does I/O or long-running work.** This is not enforced by the compiler — it is a firm Go convention. The convention exists so that cancellation can propagate all the way down the call stack without any layer needing to know about the layers above it.

```go
// Correct
func (c *clientImpl) Search(ctx context.Context, title, artist string) ([]Recording, error)

// Never do this — context stored in a struct is an anti-pattern
type clientImpl struct {
    ctx context.Context // wrong
}
```

Context stored in a struct can't be cancelled per-request — it is frozen at construction time. Per-function contexts let each call carry its own cancellation scope.

---

## 4. The `select` statement

`select` is like a `switch`, but each case is a channel operation. It blocks until one of the cases can proceed, then executes that case. If multiple cases are ready at the same time, Go picks one at random.

```go
select {
case v := <-ch1:       // receives from ch1 when a value is available
    fmt.Println(v)
case ch2 <- 42:        // sends to ch2 when ch2 is ready to receive
    fmt.Println("sent")
case <-time.After(1 * time.Second): // fires after 1 second
    fmt.Println("timed out")
}
```

`time.After(d)` returns a channel (`<-chan time.Time`) that receives exactly one value after duration `d`. It is a convenient way to implement a timeout or a delay inside a `select`.

The `select` in `withRetry`:

```go
select {
case <-time.After(delay):  // wait the full delay, then continue retrying
case <-ctx.Done():          // context was cancelled — stop immediately
    return ctx.Err()
}
```

This waits for whichever happens first: the delay expires, or the context is cancelled. Without `select`, you would use `time.Sleep(delay)`, but `time.Sleep` cannot be interrupted — it always sleeps for the full duration even if the context was cancelled a millisecond after the sleep started.

---

## 5. Walking through the code

### Setup

```go
withRetry(ctx, 3, 2*time.Second, fn)
// maxAttempts=3, delay=2s
// i will be: 0, 1, 2
```

The last iteration check `if i < maxAttempts-1` (`if i < 2`) ensures the delay is only inserted **between** attempts, never after the last one. Without this guard, the function would always wait 2 seconds after the final failure before returning — which is pointless and surprising.

---

### Scenario 1 — success on the first attempt

```
i=0: fn() → nil
     return nil  ✓
```

`fn()` succeeds. The function returns `nil` immediately. No delay, no further iterations.

---

### Scenario 2 — two failures, then success

```
i=0: fn() → error("connection refused")
     i < 2 → true
     select: time.After(2s) fires → continue

i=1: fn() → error("connection refused")
     i < 2 → true
     select: time.After(2s) fires → continue

i=2: fn() → nil
     return nil  ✓
```

The function tries three times with 2-second pauses between each. It returns `nil` after the third attempt.

---

### Scenario 3 — all attempts fail

```
i=0: fn() → error("timeout")
     i < 2 → true, wait 2s

i=1: fn() → error("timeout")
     i < 2 → true, wait 2s

i=2: fn() → error("timeout")
     i < 2 → false (no delay)
     loop ends

return err  ("timeout")
```

The `err` variable was declared with `var err error` before the loop. Each failed `fn()` overwrites it. After the loop, the last error is returned.

---

### Scenario 4 — context cancelled during `fn()`

This is the most important scenario to understand.

```
i=0: fn() starts an HTTP request with ctx
     → the HTTP client sees ctx.Done() close mid-flight
     → the HTTP call returns an error: "context canceled"
     fn() returns that error
     i < 2 → true

     select:
       time.After(2s): not ready yet
       ctx.Done():     already closed (context is already cancelled)
       → ctx.Done() wins immediately
     return ctx.Err()  ("context canceled")
```

When the context is cancelled, `ctx.Done()` is a closed channel. A closed channel is always immediately ready to receive. So even though `time.After(2s)` hasn't fired, the `select` picks `ctx.Done()` and the function returns right away. There is no further waiting.

If `fn()` itself passes `ctx` to its underlying operations (as it should), it will also return promptly on cancellation — you don't have to do anything extra.

---

### Scenario 5 — context cancelled during the delay

```
i=0: fn() → error
     i < 2 → true

     select:
       time.After(2s): would fire in 1.8 seconds
       ctx.Done():     fires after 0.3 seconds (context cancelled by caller)
       → ctx.Done() wins at 0.3s
     return ctx.Err()
```

The 2-second delay is interrupted after 0.3 seconds. The function does not wait the remaining 1.7 seconds. This is the whole point of using `select` instead of `time.Sleep`.

---

## 6. Variations

The function as written is minimal and sufficient for the MusicBrainz client. Here are extended versions for reference.

### Variation 1 — exponential backoff

Fixed delays are predictable but can overwhelm a struggling service: if 100 clients all retry after exactly 2 seconds, you send a burst of 100 requests at second 2, second 4, and so on. **Exponential backoff** increases the delay after each failure:

```go
func withRetry(ctx context.Context, maxAttempts int, initialDelay time.Duration, fn func() error) error {
    var err error
    delay := initialDelay
    for i := range maxAttempts {
        err = fn()
        if err == nil {
            return nil
        }
        if i < maxAttempts-1 {
            select {
            case <-time.After(delay):
            case <-ctx.Done():
                return ctx.Err()
            }
            delay *= 2 // double the delay each time: 1s, 2s, 4s, 8s...
        }
    }
    return err
}
```

With `initialDelay=1s` and `maxAttempts=4` the delays are 1s, 2s, 4s — the fourth attempt has no delay after it.

### Variation 2 — jitter

Even with exponential backoff, multiple clients that started at the same time will retry at the same times. Adding a random **jitter** spreads the retries out:

```go
import "math/rand"

delay = delay + time.Duration(rand.Int63n(int64(delay/2)))
// e.g. instead of exactly 2s, wait somewhere between 2s and 3s
```

This is called "full jitter" or "decorrelated jitter" and is standard practice for high-traffic services. AWS has a good write-up on it. For the MusicBrainz client (1 req/sec, single user) jitter is unnecessary.

### Variation 3 — retry only on specific errors

Sometimes you only want to retry on transient errors (network failures, 429, 503), not on permanent ones (404, 400, bad input). Define a predicate:

```go
func withRetry(ctx context.Context, maxAttempts int, delay time.Duration, shouldRetry func(error) bool, fn func() error) error {
    var err error
    for i := range maxAttempts {
        err = fn()
        if err == nil {
            return nil
        }
        if !shouldRetry(err) {
            return err // don't retry — permanent error
        }
        if i < maxAttempts-1 {
            select {
            case <-time.After(delay):
            case <-ctx.Done():
                return ctx.Err()
            }
        }
    }
    return err
}
```

Usage:

```go
err := withRetry(ctx, 3, 2*time.Second, func(err error) bool {
    var httpErr *HTTPError
    if errors.As(err, &httpErr) {
        return httpErr.StatusCode == 429 || httpErr.StatusCode >= 500
    }
    return true // retry on network errors
}, fn)
```

### Variation 4 — logging on each retry

Useful during development or in production to see how often retries happen:

```go
func withRetry(ctx context.Context, maxAttempts int, delay time.Duration, fn func() error) error {
    var err error
    for i := range maxAttempts {
        err = fn()
        if err == nil {
            return nil
        }
        if i < maxAttempts-1 {
            log.Printf("attempt %d/%d failed: %v — retrying in %s", i+1, maxAttempts, err, delay)
            select {
            case <-time.After(delay):
            case <-ctx.Done():
                return ctx.Err()
            }
        }
    }
    log.Printf("all %d attempts failed: %v", maxAttempts, err)
    return err
}
```

---

## 7. Why not `time.Sleep`?

You might wonder why the function uses `select` instead of the simpler `time.Sleep`:

```go
// Simpler, but wrong
time.Sleep(delay)
```

`time.Sleep` does not accept a context. It always sleeps for the full duration. If the context is cancelled while the goroutine is sleeping, the goroutine stays blocked until the sleep ends, then checks the error from `fn()` on the next iteration — and only then discovers the context was cancelled.

In the worst case with `maxAttempts=3` and `delay=2s`, a cancelled context could keep the goroutine alive for an extra 2 seconds after cancellation. In a server handling many concurrent requests, this adds up.

The `select`-based version exits within microseconds of cancellation.

---

## 8. External libraries

Several Go libraries implement retry logic:

- [`cenkalti/backoff`](https://github.com/cenkalti/backoff) — exponential backoff with jitter, configurable retry conditions
- [`avast/retry-go`](https://github.com/avast/retry-go) — clean API, supports per-attempt callbacks and custom backoff strategies
- [`failsafe-go`](https://github.com/failsafe-go/failsafe-go) — full resilience toolkit: retry, circuit breaker, rate limiter, timeout

These libraries are well-maintained and worth reaching for when:
- You need exponential backoff with proper jitter out of the box
- You need a circuit breaker (stop retrying after N consecutive failures; resume after a cooldown period)
- The retry logic has become complex enough that a library is clearer than hand-rolled code

For this project they are not used because:
1. The hand-rolled function is 15 lines and does exactly what is needed — no more.
2. Adding a library means new transitive dependencies to audit and update.
3. The fixed delay between retries is appropriate for a 1 req/sec rate-limited API — the purpose of the retry is to handle transient blips, not to manage load.

A useful rule of thumb: bring in a resilience library when you need a feature it provides (circuit breaking, jitter) that you would otherwise have to implement yourself. Don't bring it in to replace code that is already shorter than the library's import path.

---

## Summary

| Concept | One line |
|---|---|
| `func() error` parameter | A function passed as a value — decouples the retry logic from the operation |
| `for i := range n` | Go 1.22 integer range — equivalent to `for i := 0; i < n; i++` |
| `context.Context` | A cancellation signal passed down the call stack; forms a tree |
| `ctx.Done()` | A channel closed when the context is cancelled |
| `ctx.Err()` | The reason for cancellation: `Canceled` or `DeadlineExceeded` |
| `select` | Waits on multiple channel operations; picks the first ready one |
| `time.After(d)` | Returns a channel that receives once after duration `d` |
| `time.Sleep` vs `select` | `Sleep` cannot be interrupted; `select` exits immediately on cancellation |
