package cadastre

import "fmt"
import "io"
import "bytes"
import "encoding/json"
import "database/sql"
import _ "github.com/go-sql-driver/mysql"

// Defines a single event at a given point in time in the MySQL process list. It includes
// the query ID (event ID), how long the query has been running, who is running it and from
// where, the sttus of the query, the actual query itself, and metadata such as rows sent,
// examined and read, where applicable.
type Event struct {
	EventID      int64  `json:"id"`
	TimeElapsed  int64  `json:"timeElapsed"`
	Host         string `json:"host"`
	Database     string `json:"database"`
	User         string `json:"user"`
	Command      string `json:"command"`
	Status       string `json:"status"`
	SQL          string `json:"sql"`
	RowsSent     int64  `json:"rowsSent"`
	RowsExamined int64  `json:"rowsExamined"`
	RowsRead     int64  `json:"rowsRead"`
}

// A collection of events that represent a complete point in time view of the MySQL process list.
type Snapshot struct {
	Events []Event `json:"events"`
}

func (me *Snapshot) TakeSnapshot(server Server) error {
	// Start with a fresh array.
	me.Events = []Event{}

	// Try to connect to our host.
	databaseConnection, err := sql.Open("mysql", server.DataSourceName)
	if err != nil {
		return fmt.Errorf("Caught an error while trying to connect to the target MySQL server! %s", err)
	}
	defer databaseConnection.Close()

	// Make sure our connection is valid.
	err = databaseConnection.Ping()
	if err != nil {
		return fmt.Errorf("Unable to connect to target MySQL server! %s", err)
	}

	// Try and get the process list.
	rows, err := databaseConnection.Query("SHOW FULL PROCESSLIST")
	if err != nil {
		return fmt.Errorf("Caught an error while querying the target MySQL server for the process list! %s", err)
	}

	// Get the column list returned for this query.
	rowColumns, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("Caught an error while trying to grab the row columns! %s", err)
	}

	if len(rowColumns) != 8 && len(rowColumns) != 12 {
		return fmt.Errorf("Unsupported table format for SHOW FULL PROCESSLIST: expected 8 or 12 columns, got back %d", len(rowColumns))
	}

	// Make holders for all the values we might get back.
	var eventId int64
	var user string
	var host string
	var database sql.NullString
	var command string
	var timeElapsed int64
	var status sql.NullString
	var sql sql.NullString
	var timeMs int64
	var rowsExamined int64
	var rowsSent int64
	var rowsRead int64

	// Go through each row, converting it to an Event object.
	for rows.Next() {
		// Create a new event object.
		event := Event{}

		switch len(rowColumns) {
		case 12:
			// This should be results from Percona Server.
			err = rows.Scan(&eventId, &user, &host, &database, &command, &timeElapsed, &status, &sql, &timeMs, &rowsExamined, &rowsSent, &rowsRead)
			if err != nil {
				return fmt.Errorf("Caught an error while parsing the response from SHOW FULL PROCESSLIST: %s", err)
			}
		case 8:
			// This should be results from stock MySQL.
			err = rows.Scan(&eventId, &user, &host, &database, &command, &timeElapsed, &status, &sql)
			if err != nil {
				return fmt.Errorf("Caught an error while parsing the response from SHOW FULL PROCESSLIST: %s", err)
			}
		}

		// Populate our event object.
		event.EventID = eventId
		event.User = user
		event.Host = host
		event.Database = database.String
		event.Command = command
		event.TimeElapsed = timeElapsed
		event.Status = status.String
		event.SQL = sql.String

		// If this is Percona Server, pull out the row counts for the query, too.
		if len(rowColumns) == 12 {
			event.RowsExamined = rowsExamined
			event.RowsSent = rowsSent
			event.RowsRead = rowsRead
		}

		me.Events = append(me.Events, event)
	}

	return nil
}

func (me *Snapshot) WriteTo(w io.Writer) error {
	// Create our JSON encoder, because that's how we want to serialize ourselves.
	buf := bytes.NewBuffer([]byte{})
	jsonEncoder := json.NewEncoder(buf)

	// Write ourselves out as JSON to the buffer.
	if err := jsonEncoder.Encode(me); err != nil {
		return fmt.Errorf("Encountered an error during serialization! %s", err)
	}

	// Write our buffer to our input writer.
	if _, err := buf.WriteTo(w); err != nil {
		return fmt.Errorf("Error while writing serialized snapshot! %s", err)
	}

	// All good!
	return nil
}

func NewSnapshotFromReader(r io.Reader) (*Snapshot, error) {
	// Create a buffer to hold the JSON we pull in.
	buf := bytes.NewBuffer([]byte{})

	// Read it in!
	if _, err := buf.ReadFrom(r); err != nil {
		return nil, fmt.Errorf("Error while reading in serialized snapshot! %s", err)
	}

	// Now that we have it, decode it and cast it to our object.
	jsonDecoder := json.NewDecoder(buf)

	newSnapshot := &Snapshot{}
	if err := jsonDecoder.Decode(newSnapshot); err != nil {
		return nil, fmt.Errorf("Error while deserializing snapshot! %s", err)
	}

	return newSnapshot, nil
}
