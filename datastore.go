package cadastre

import "time"

// Defines the basic interface for the storage engine.
type DataStore interface {
    // Allows the storage engine time before serving requests to initialize itself, spin up any
    // background workers, etc.
    Initialize() (error)

    // Retrieve the latest snapshot for the given identifier (host).  If no data could be found, or
    // an error occured, the return value will be nil and the error will indicate what went wrong.
    RetrieveLatest(identifier string) (*Snapshot, error)

    // Retrieve any snapshots for the given identifier (host) that occured between the specified start
    // and end time.  If no data could be found, or an error occured, the return value will be nil and
    // the error will indicate what went wrong.
    RetrieveRange(identifier string, start time.Time, end time.Time) (map[time.Time] *Snapshot, error)

    // Persists a snapshot for the given identifier to the underlying storage engine.
    Persist(identifier string, timestamp time.Time, value *Snapshot) (error)
}
