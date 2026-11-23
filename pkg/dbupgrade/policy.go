package dbupgrade

// BackupFailure says what the startup routine does when the pre-migration
// backup cannot be made or fails its check (BR-038 AC-8, owner question Q-8).
type BackupFailure int

const (
	// Refuse applies no migration, leaves the database unchanged and exits.
	Refuse BackupFailure = iota + 1
	// MigrateWithoutBackup prints a warning and migrates anyway.
	MigrateWithoutBackup
)

// MigrationFailure says what the startup routine does when a migration fails
// after a good backup was made (BR-038 AC-10, owner question Q-9).
type MigrationFailure int

const (
	// Keep leaves the database as the failed migration left it, keeps the
	// backup and reports it.
	Keep MigrationFailure = iota + 1
	// Restore puts the backup back, then exits.
	Restore
)

// The owner's answers to Q-8 and Q-9. Changing an answer means changing one
// of these two constants and nothing else (ADR-006 D-7).
const (
	DefaultOnBackupFailure    = Refuse
	DefaultOnMigrationFailure = Keep
)

// Policy holds the two switches. Use DefaultPolicy unless a test needs a
// specific value.
type Policy struct {
	OnBackupFailure    BackupFailure
	OnMigrationFailure MigrationFailure
}

// DefaultPolicy returns the policy the owner's answers currently select.
func DefaultPolicy() Policy {
	return Policy{OnBackupFailure: DefaultOnBackupFailure, OnMigrationFailure: DefaultOnMigrationFailure}
}
