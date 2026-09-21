# Logging Guidelines

> Zerolog only. Request-scoped fields come from middleware, not ad-hoc `log.Print`.

---

## Overview

`internal/logger.Open(cfg, ctx)` builds one log stream from a `config.StreamConfig`. The HTTP process opens **five** streams (each may disable a sink, but not both):

| Stream | Source YAML | Context fields |
|--------|-------------|----------------|
| Process system | `config.yaml` `log` | `cat=system` |
| Admin system | `admin.yaml` `log.system` | `cat=system`, `plane=admin`, `mod=http` |
| Admin access | `admin.yaml` `log.access` | `cat=access`, `plane=admin` |
| Client system | `client.yaml` `log.system` | `cat=system`, `plane=client`, `mod=http` |
| Client access | `client.yaml` `log.access` | `cat=access`, `plane=client` |

Console sink: `zerolog.ConsoleWriter` on stderr (`console.no_color` → `NoColor`; default colored). Do not invent `auto\|always\|never`. File sink: JSON via lumberjack (size rotation). Empty `file.dir` or `file.filename` while `file.enabled` is a hard error — never fall back to `os.TempDir`. Both sinks off → `Open` error; process exits before listen.

`zerolog/log.Logger` is assigned the process system logger so stray globals cannot bypass files. Controllers must not create a second global logger.

---

## Log Levels

| Level | When |
|-------|------|
| debug | Local development (`*-example.yaml` default may be debug; Viper default is info); Gin `[GIN-debug]` (`mod=gin`) |
| info | Successful listen, HTTP access line, other gin.DefaultWriter lines (`mod=gin`) |
| warn | Recoverable degradation (runtime DB ping failed while process still listens, storage open failed but process still listening), GORM slow SQL (≥200ms), audit write failure |
| error | Shutdown failure, panic recovered, unexpected handler/claim errors, runtime database unavailable (`mod=db` Watch), gin.DefaultErrorWriter (`mod=gin`) |
| fatal | Reserved for CLI / irrecoverable misconfiguration **before** listen (empty/invalid config, stream Open failure, `database.Open` failure, `cache.driver=redis` Ping failure). Runtime Postgres down is error + 404, not fatal. |

Invalid `level` strings fall back to `info`.

---

## Structured fields

Every stream includes `cat=system|access`. Plane streams include `plane=admin|client`. System logs also set `mod`:

| mod | Source |
|-----|--------|
| cmd | process lifecycle in `cmd/server.go` |
| db | GORM adapter (`database.Open`) and runtime Watch ping/reconnect |
| storage | storage open/probe errors |
| cache | cache Open, Redis failover/reconnect (`internal/cache`, process system child) |
| job | worker Claim errors (not `context.Canceled` / `DeadlineExceeded`; no idle-tick spam) and unexpected handler failures |
| audit | `AuditService.Record` failure |
| gin | `gin.DefaultWriter` / `DefaultErrorWriter` |
| http | plane listen/shutdown/Recovery |

Access logs do **not** set `mod`. AccessLog fields stay `request_id`, `method`, `path`, `status`, `elapsed`, plus `cat`/`plane` from the logger context.

---

## Side channels (must stay on managed Zerolog)

- GORM: custom adapter, Warn threshold 200ms, `IgnoreRecordNotFoundError`, `Colorful=false`. Do not write GORM to stdout.
- Audit: injected `mod=audit` child, not package `zerolog/log`.
- Gin debug: `logger.NewStdWriter` on process system `mod=gin`. Lines containing `[GIN-debug]` are **Debug**; other DefaultWriter lines are **Info**. `gin.DefaultErrorWriter` uses `logger.NewErrorWriter` (**Error**). Do not map every gin line to Info.
- `kirivers admin` CLI `fmt` output is **out of scope** (human CLI, not process logs).

---

## What to Log

- Process listen address and whether TLS is enabled (plane system logger)
- DB Open failure (`error` then `fatal` before listen); migrate failure (`error`, process still listens if Open succeeded)
- Runtime DB ping failure / reconnect (`error` / `info`, `mod=db`)
- Cache Redis failover / reconnect (`error`, `mod=cache`); Open Ping failure then `fatal`
- Panic value in Recovery (not the HTML stack page)
- Admin UI `static_dir` empty or both disk and embedded `index.html` missing/not a file: `warn` on the admin system stream (`plane=admin`, `mod=http`, field `static_dir`); process still listens. Disk present → `info` `static files enabled`. Disk missing + embed has `index.html` → `info` `embedded files enabled`. Do not log directory listings or file contents. Not `fatal`.

---

## What NOT to Log

- bcrypt hashes, plaintext passwords, `url_signing_secret`, full Bearer tokens (admin session / project / CI)
- Storage secret keys, Redis `cache.redis.password`
- Raw `device_id` (use fingerprint / hash; see telemetry-privacy)
- Admin list/API never prints `PasswordHash`. CLI list prints username/id/created_at only.

---

## Scenario: Open a managed log stream

### 1. Scope / Trigger

Any new process log line, GORM output, Gin writer, or access/recovery middleware change. Infra contract: `logger.Open` + five streams + `mod`/`cat`/`plane`.

### 2. Signatures

```go
func Open(cfg config.StreamConfig, ctx zerolog.Context) (zerolog.Logger, io.Closer, error)
func NewWithWriter(w io.Writer, level zerolog.Level) zerolog.Logger // tests
func NewStdWriter(log zerolog.Logger) io.Writer                     // gin.DefaultWriter; [GIN-debug] → Debug
func NewErrorWriter(log zerolog.Logger) io.Writer                   // gin.DefaultErrorWriter → Error
```

`StreamConfig`: `level`, `console.enabled`, `console.no_color`, `file.enabled`, `file.dir`, `file.filename`, `file.max_size_mb`, `file.max_backups`, `file.max_age_days`, `file.compress`, `file.local_time`.

HTTP server opens five streams (see Overview table). `kirivers admin` does not open streams (CLI `fmt` only).

### 3. Contracts

| Item | Contract |
|------|----------|
| Console | `zerolog.ConsoleWriter` → stderr. `no_color` maps to `NoColor`. No `auto\|always\|never`. |
| File | JSON via lumberjack; `Filename = filepath.Join(dir, filename)`. |
| Context | Caller must already put `cat` (and `plane` on plane streams) on `ctx`. |
| Access | AccessLog fields `request_id`/`method`/`path`/`status`/`elapsed`; no `mod`; `path` without query. |
| Global | After Open, `zerolog/log.Logger` = process system logger (`cat=system`, no `mod`). |
| GORM | `database.Open(dsn, log)` adapter: effective Warn (slow ≥200ms / Error). Do not forward GORM Info (would dump SQL when process level is debug). |
| Env | Stream keys under `KIRIVERS_` / `KIRIVERS_ADMIN_` / `KIRIVERS_CLIENT_` (see quality-guidelines BindEnv). |

### 4. Validation & Error Matrix

| Condition | Result |
|-----------|--------|
| Empty / whitespace / invalid `level` | `info` (empty is Zerolog `NoLevel` if passed through — must special-case) |
| `console.enabled` and `file.enabled` both false | `Open` error; process exits before listen |
| `file.enabled` and `dir` or `filename` empty (after trim) | `Open` error; **never** lumberjack default TempDir |
| `file.enabled` and `MkdirAll` fails | `Open` error |
| File disabled | nop closer; console-only |
| GORM `Info` (routine SQL) | discarded; only Slow/Error/Warn |

### 5. Good/Base/Bad Cases

- Good: example YAML both sinks on; files under `./logs` (`kirivers.log`, `admin-system.log`, `admin-access.log`, `client-system.log`, `client-access.log`).
- Base: disable file in Docker, leave console on; stderr still ConsoleWriter.
- Bad: `zerolog.ParseLevel("")` without empty check; `lumberjack.Logger{Filename: ""}`; `log.New(os.Stdout, …)` in `database.Open`; `github.com/rs/zerolog/log` in audit without assigning `log.Logger`.

### 6. Tests Required

- Empty level → info (not NoLevel / not every debug line).
- Both sinks off → error; empty dir or filename with file enabled → error; mkdir failure → error.
- File JSON contains `mod`/`cat` (and `plane` when set); access logger has no `mod`.
- Prefix isolation: `KIRIVERS_ADMIN_LOG_SYSTEM_LEVEL` must not set client/system stream level.
- GORM adapter: Warn/Error appears in injected buffer; no plaintext secrets.
- AccessLog: request fields present; `path` has no query; no raw `device_id`.

### 7. Wrong vs Correct

#### Wrong

```go
lvl, _ := zerolog.ParseLevel(cfg.Level) // "" → NoLevel → logs everything
lj := &lumberjack.Logger{Filename: cfg.File.Filename} // empty → os.TempDir()
```

#### Correct

```go
log, closer, err := logger.Open(cfg, zerolog.New(io.Discard).With().Str(logger.FieldCat, logger.CatSystem))
if err != nil {
    return err // fatal before listen
}
defer closer.Close()
```

---

## Common Mistake: empty level is NoLevel

**Symptom**: `level:` omitted or `level: ""` dumps debug/GORM SQL.

**Cause**: `zerolog.ParseLevel("")` returns `NoLevel`, which does not filter.

**Fix**: trim/lower then treat empty as `info` before `ParseLevel`.

**Prevention**: keep the `parseLevel` empty-string branch and the empty-level logger test.
