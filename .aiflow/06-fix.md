# 06 — fix

## Defect

`Sync` discarded the errors from both backup writes and then overwrote the live config anyway, so a failed backup silently destroyed the user's original `~/.ssh/config` while returning `nil` — `internal/sshconfig/sshconfig.go:98,101`.

(The report called the function `Apply`; the function at those lines is `Sync`. Same code, same defect.)

## Reproducing test

`TestSyncFailsWhenBackupCannotBeWritten` in `internal/sshconfig/backup_test.go`.

It writes a hand-written config, occupies the backup path `config.sshm.bak` with a directory so `os.WriteFile` cannot succeed there, then calls `Sync`. Before the fix:

```
=== RUN   TestSyncFailsWhenBackupCannotBeWritten
    backup_test.go:27: Sync returned nil even though the backup could not be written
--- FAIL: TestSyncFailsWhenBackupCannotBeWritten (0.00s)
```

The test asserts both halves of the promise: `Sync` returns an error, and the live config still holds the original bytes. Asserting the file content means the test fails on the destructive behaviour itself, not just on the missing error value.

The directory-on-the-backup-path trick was chosen over `chmod`-ing the parent read-only because it fails deterministically even when the suite runs as root in CI.

## Change

`Sync` now returns a wrapped error from either backup write instead of discarding it, so the real config is never overwritten when its backup could not be made.

## Verification

- `go test ./...` — all green.
- `go vet ./...` — clean.
- Diff touches one file, 7 insertions / 2 deletions. No existing test was modified.

## Not taken

Only one defect was reported, and it was fixed.

One adjacent hole was found while writing the test and deliberately left alone, as it is a separate defect rather than part of this one: at `sshconfig.go:97` the pristine-backup guard is `if _, err := os.Stat(orig); os.IsNotExist(err)`. Any `Stat` error that is *not* "not exist" — `ENAMETOOLONG`, `EACCES` on the directory, or the path existing as a directory — makes `Sync` skip the pristine backup entirely and treat it as already present. The "always recoverable" promise in the comment above it is still not fully enforced in those cases. Fixing it would mean changing the existence check rather than the error handling, which is outside this one-defect scope and worth reporting separately.
