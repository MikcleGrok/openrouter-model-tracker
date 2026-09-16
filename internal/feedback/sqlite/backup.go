package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"
)

// DefaultBackupRetain is how many backup files Backup keeps when retain is
// not explicitly set (<= 0 passed by a caller). Plan 9.2 asks only for "an
// ограниченное число копий с датой" without fixing the number; 5 is a small,
// deliberately conservative default.
const DefaultBackupRetain = 5

// backupFilePattern matches this package's backup filenames:
// feedback-v<schema-version>-<UTC timestamp>.sqlite. It is used only to
// recognize this Store's own backups for pruning (pruneOldBackups) — it
// never parses the timestamp back into a time.Time, since the filename's
// lexicographic order already matches chronological order for this format.
const backupFileGlob = "feedback-v*-*.sqlite"

// Backup takes a consistent, schema-tagged copy of the live database into
// dir and returns its path. It never does a plain filesystem copy of the
// live file (unsafe under WAL: plan 5.1's "WAL и backup не считать
// безопасным основанием... не гарантирует reapply"; plan 9.2's "не простое
// копирование WAL-состояния"); instead it uses SQLite's own `VACUUM INTO`,
// which produces a single, fully checkpointed, internally consistent
// snapshot file regardless of the live database's current journal mode —
// the "SQLite online backup либо согласованная копия после checkpoint"
// plan 9.2 calls for, without needing the C sqlite3_backup_* API modernc.org/
// sqlite does not expose through database/sql.
//
// The sequence is: VACUUM INTO a temporary file in dir, fsync that file,
// atomically rename it into its final, schema-tagged name (plan 9.2:
// "писать во временный файл, fsync, затем атомарно rename"), fsync the
// directory entry, then verify the result is readable by a brand new SQLite
// connection (plan 9.2: "проверять, что файл читается отдельным SQLite
// connection") before pruning older backups beyond retain (<= 0 means
// DefaultBackupRetain).
//
// This same method is used both for an operator-triggered backup (e.g.
// before a destructive migration or a release rollback, plan 9.2) and, from
// RunCleanup, as the post-commit privacy-deletion backup — the two are the
// same operation with no privacy-specific branch: a backup taken after
// DeleteIdentity's commit simply no longer contains the deleted identity's
// rows, because VACUUM INTO reads the live database as it is right now.
func (s *Store) Backup(ctx context.Context, dir string, retain int) (string, error) {
	if retain <= 0 {
		retain = DefaultBackupRetain
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("sqlite: backup: create dir %s: %w", dir, err)
	}

	version, err := s.SchemaVersion(ctx)
	if err != nil {
		return "", fmt.Errorf("sqlite: backup: %w", err)
	}

	timestamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	finalName := fmt.Sprintf("feedback-v%d-%s.sqlite", version, timestamp)
	finalPath := filepath.Join(dir, finalName)
	tmpPath := finalPath + ".tmp"

	// Best-effort: clear a stale tmp file left by a previous crashed
	// attempt at exactly this (unlikely, nanosecond-timestamped) name.
	_ = os.Remove(tmpPath)

	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("sqlite: backup: vacuum into %s: %w", tmpPath, err)
	}

	if err := fsyncFile(tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("sqlite: backup: fsync %s: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("sqlite: backup: rename into place: %w", err)
	}

	if err := fsyncDir(dir); err != nil {
		return finalPath, fmt.Errorf("sqlite: backup: fsync dir %s: %w", dir, err)
	}

	if err := verifyReadable(ctx, finalPath); err != nil {
		// A backup that fails its own readability check must not be left on
		// disk: besides being useless, it would still count toward
		// pruneOldBackups' retention and could push a genuinely good backup
		// out ahead of a corrupt one.
		_ = os.Remove(finalPath)
		return "", fmt.Errorf("sqlite: backup: verify %s: %w", finalPath, err)
	}

	if err := pruneOldBackups(dir, retain); err != nil {
		return finalPath, fmt.Errorf("sqlite: backup: prune old backups: %w", err)
	}

	return finalPath, nil
}

// Restore atomically replaces destPath's file with backupPath's contents —
// the file-level half of plan 9.2's restore runbook ("остановить сервер,
// сохранить текущую DB, заменить backup, запустить сервер и проверить
// schema/health"): stopping the server and starting it back up, and any
// caller-side decision to keep destPath's pre-restore content first (e.g. by
// calling Backup against the still-running Store before stopping it), are
// this package's caller's job, not this function's — Restore must not be
// called against a path a Store still has open.
//
// It refuses to touch destPath at all if backupPath does not open as a
// valid SQLite database. It then removes any stale -wal/-shm sidecar files
// left over from destPath's previous life -- before the rename, not after:
// the old main file is being discarded either way, and leaving its WAL in
// place even briefly after the new content is renamed into destPath would
// let SQLite replay that unrelated WAL over the just-restored content,
// including inside the verifyReadable call below. It then copies backupPath
// into a temporary file next to destPath, fsyncs and atomically renames that
// into destPath, and finally re-verifies destPath itself opens.
func Restore(ctx context.Context, backupPath, destPath string) error {
	if err := verifyReadable(ctx, backupPath); err != nil {
		return fmt.Errorf("sqlite: restore: source %s is not a readable SQLite database: %w", backupPath, err)
	}

	// A removal error here (anything other than "the sidecar doesn't exist")
	// is a hard failure, not best-effort: silently leaving a stale -wal/-shm
	// behind is exactly the corruption hazard this ordering exists to avoid.
	if err := removeSidecarFile(destPath + "-wal"); err != nil {
		return fmt.Errorf("sqlite: restore: remove stale %s-wal: %w", destPath, err)
	}
	if err := removeSidecarFile(destPath + "-shm"); err != nil {
		return fmt.Errorf("sqlite: restore: remove stale %s-shm: %w", destPath, err)
	}

	tmpPath := destPath + ".restoring.tmp"
	if err := copyFile(backupPath, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("sqlite: restore: copy %s: %w", backupPath, err)
	}
	if err := fsyncFile(tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("sqlite: restore: fsync %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("sqlite: restore: rename into place: %w", err)
	}
	if err := fsyncDir(filepath.Dir(destPath)); err != nil {
		return fmt.Errorf("sqlite: restore: fsync dir: %w", err)
	}

	if err := verifyReadable(ctx, destPath); err != nil {
		return fmt.Errorf("sqlite: restore: verify %s after restore: %w", destPath, err)
	}
	return nil
}

// removeSidecarFile removes path, treating "it does not exist" (the normal
// case -- most databases are not mid-WAL-checkpoint when backed up) as
// success rather than an error.
func removeSidecarFile(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// verifyReadable opens path as a brand new, plain SQLite connection
// (distinct from any Store already open elsewhere in the process) and runs
// a trivial query, satisfying plan 9.2's "проверять, что файл читается
// отдельным SQLite connection" for both Backup and Restore. Unlike Open, it
// applies none of Store's connection parameters (no _journal_mode=WAL in
// particular): switching journal mode is itself a write that would leave
// -wal/-shm sidecar files next to a backup file this function should only
// be reading, so this check is deliberately a plain, side-effect-free
// connection instead of a full Store.
func verifyReadable(ctx context.Context, path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("sqlite: open %s for verification: %w", path, err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("sqlite: ping %s for verification: %w", path, err)
	}
	var one int
	if err := db.QueryRowContext(ctx, `SELECT 1`).Scan(&one); err != nil {
		return fmt.Errorf("sqlite: readability check query: %w", err)
	}
	return nil
}

// pruneOldBackups removes this Store's own backup files in dir beyond the
// most recent retain, oldest first. Filenames sort lexicographically in
// chronological order (feedback-v<N>-<UTC RFC3339-ish timestamp>.sqlite), so
// no filename parsing is needed to order them.
func pruneOldBackups(dir string, retain int) error {
	matches, err := filepath.Glob(filepath.Join(dir, backupFileGlob))
	if err != nil {
		return fmt.Errorf("sqlite: list backups: %w", err)
	}
	sort.Strings(matches)
	if len(matches) <= retain {
		return nil
	}
	for _, stale := range matches[:len(matches)-retain] {
		if err := os.Remove(stale); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("sqlite: remove old backup %s: %w", stale, err)
		}
	}
	return nil
}

// copyFile copies src to dst, creating/truncating dst. It does not fsync —
// callers that need durability call fsyncFile on dst afterward.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

// fsyncFile fsyncs the file at path.
func fsyncFile(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

// fsyncDir fsyncs the directory at path, so a rename into it is durable
// before this function returns. Windows does not support opening a
// directory as a syncable handle, so this is a deliberate no-op there — the
// rename itself (NTFS journals directory-entry updates) is the practical
// durability guarantee on that platform.
func fsyncDir(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	d, err := os.Open(path)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
