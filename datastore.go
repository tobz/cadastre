package cadastre

import "time"

// Defines the basic interface for the storage engine.
type DataStore interface {
	// Allows the storage engine time before serving requests to initialize itself, spin up any
	// background workers, etc.
	Initialize() error

	// Retrieve the snapsot for the given timestamp.  Returns nil if the snapshot for the given
	// timestamp can't be found, and an error if any errors were encountered.
	Retrieve(identifier string, timestamp time.Time) (*Snapshot, error)

	// Persists a snapshot for the given identifier to the underlying storage engine.
	Persist(identifier string, timestamp time.Time, value *Snapshot) error
}
