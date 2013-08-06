package cadastre

import "time"

// Defines the basic interface for the storage engine.
type DataStore interface {
	// Allows the storage engine time before serving requests to initialize itself, spin up any
	// background workers, etc.
	Initialize() error

	// Retrieve the snapshot for the given timestamp.  Returns nil if the snapshot for the given
	// timestamp can't be found, and an error if any errors were encountered.
	RetrieveSnapshot(identifier string, timestamp time.Time) (*Snapshot, error)

	// Retrieve the thread counts for snapshots taken within the given time period.  The time period
	// is calculated as anything on the same day as the given datestamp - only the date matters. Returns
	// nil if there are no counts for the given time period, and an error if any errors were encountered.
	RetrieveCounts(identifier string, datestamp time.Time) (*Counts, error)

	// Persists a snapshot for the given identifier to the underlying storage engine.
	Persist(identifier string, value *Snapshot) error
}
