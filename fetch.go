package cadastre

import "time"
import "sync"
import "log"

// Represents the fetcher, which runs continuously, sucking up data from the specified hosts
// and persisting it to the storage engine for later retrieval.
type Fetcher struct {
	Configuration *Configuration
	running       bool
	fetchGuard    sync.WaitGroup
}

func (me *Fetcher) Start() error {
	me.running = false

	processListFetcher := func(server Server) {
		// Increment the fetcher guard.
		me.fetchGuard.Add(1)

		for {
			// See if we should still be running.
			if !me.running {
				break
			}

			// Wait our fetch interval before we proceed.
			time.Sleep(me.Configuration.FetchInterval)

			// Check again to see if we should be running, as we could have just sat through
			// a moderately-long fetch interval and have the process waiting on us to stop.
			if !me.running {
				break
			}

			snapshotTimestamp := time.Now()
			snapshot := &Snapshot{}
			if err := snapshot.TakeSnapshot(server); err != nil {
				log.Printf("error: failed to take snapshot for host '%s'! %s", server.InternalName, err)
			} else {
				// Persist it to our datastore.
				me.Configuration.Storage.Persist(server.InternalName, snapshotTimestamp, snapshot)
			}
		}

		// All done, so decrement the fetcher guard.
		me.fetchGuard.Done()
	}

	// Set ourselves as running.
	me.running = true

	var fetcherCount uint64

	// Create a fetcher routine for each server we want to monitor.
	for _, server := range me.Configuration.Servers {
		fetcherCount++

		go processListFetcher(server)
	}

	log.Printf("Launched %d fetcher(s) to poll databases...", fetcherCount)

	return nil
}

func (me *Fetcher) Stop() error {
	// Alert all goroutines that we're done running.
	me.running = false

	// Wait for all fetchers to stop.
	me.fetchGuard.Wait()

	return nil
}
