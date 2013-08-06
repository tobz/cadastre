package cadastre

import "os"
import "time"
import "fmt"
import "log"
import "regexp"
import "path/filepath"
import "bufio"
import "strings"

type FileStore struct {
	DataDirectory    string
	absDataDirectory string

	RetentionPeriod time.Duration
}

func (me *FileStore) Initialize() error {
	// Make sure our data directory is a qualified, absolute path.
	absDataDirectory, err := filepath.Abs(me.DataDirectory)
	if err != nil {
		return fmt.Errorf("Error normalizing the data directory path! %s", err)
	}

	me.absDataDirectory = absDataDirectory

	return nil
}

func (me *FileStore) RetrieveSnapshot(identifier string, timestamp time.Time) (*Snapshot, error) {
	// First, parse our timestamp into YYYY-MM-DD and then YYYY-MM-DD-hh-mm-ss so we can build a filepath to try and access.
	baseDirectoryName := timestamp.Format("2006-01-02")
	snapshotFileName := timestamp.Format("2006-01-02-15-04-05") + ".spl"

	finalDirectoryPath := filepath.Join(me.absDataDirectory, baseDirectoryName, identifier)

	// See if the directory pointed to by this timestamp actually exists.
	if !doesDirectoryExist(finalDirectoryPath) {
		// We didn't find anything.... that's weird.  Maybe we only just started?  In any case, it could
		// be a case of legitimately having no data, so just return nil.
		return nil, fmt.Errorf("No data available for the given time period.")
	}

	finalFilePath := filepath.Join(finalDirectoryPath, snapshotFileName)

	// Now see if the file is there.
	if !doesFileExist(finalFilePath) {
		// Well that's weird, but again, this could be an erroneous request and the timestamp could
		// be in the future or the client could be buggy or whatever.  Just return nil.
		return nil, fmt.Errorf("No data available for the given time period.")
	}

	return hydrateSnapshotFromFile(finalFilePath)
}

func (me *FileStore) RetrieveCounts(identifier string, datestamp time.Time) (*Counts, error) {
	counts := make([]Count, 0)

	// First, parse our datestamp into YYYY-MM-DD so we can build a filepath to try and access.
	baseDirectoryName := datestamp.Format("2006-01-02")

	finalDirectoryPath := filepath.Join(me.absDataDirectory, baseDirectoryName, identifier)

	// See if the directory pointed to by this datestamp actually exists.
	if !doesDirectoryExist(finalDirectoryPath) {
		// We didn't find anything.... that's weird.  Maybe we only just started?  In any case, it could
		// be a case of legitimately having no data, so just return nil.
		return nil, fmt.Errorf("No data available for the given time period.")
	}

	splFileRegexp := regexp.MustCompile("^\\d{4}-\\d{2}-\\d{2}-\\d{2}-\\d{2}-\\d{2}\\.spl$")

	// Now we gotta walk all the files in the directory to grab the counts.
	filepath.Walk(finalDirectoryPath, func(path string, info os.FileInfo, _ error) error {
		// We're only looking for files.
		if info.IsDir() {
			return nil
		}

		// See if the filename matches our single snapshot naming scheme.
		if splFileRegexp.MatchString(filepath.Base(path)) {
			// We should have a legitimate SPL file here, so hydrate it and get the count.
			snapshot, err := hydrateSnapshotFromFile(path)
			if err != nil {
				// Nothing to do here.  Move on to the next one.
				return nil
			}

			// Add this count to the rest.
			counts = append(counts, snapshot.GetCount())
		}

		return nil
	})

	return &Counts{Counts: counts}, nil
}

func (me *FileStore) Persist(identifier string, value *Snapshot) error {
	timestamp := time.Unix(value.Timestamp, 0)

	// Make sure the folder exists for our identifier, and within that, today's date.  Create either if they don't exist.
	topLevelTime := timestamp.Format("2006-01-02")
	lowLevelTime := timestamp.Format("2006-01-02-15-04-05")

	targetDirectory := filepath.Join(me.absDataDirectory, topLevelTime, identifier)

	if err := createDirectoryIfNotExists(targetDirectory); err != nil {
		return fmt.Errorf("Failed to create the directory to store a process list snapshot! Identifier: %s, Error: %s", identifier, err)
	}

	targetDataFile := filepath.Join(targetDirectory, lowLevelTime+".spl")

	// We have our target directory now, so let's create/open our data file and give the snapshot a writer to serialize itself to.
	targetFileHandle, err := os.OpenFile(targetDataFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0774)
	if err != nil {
		return fmt.Errorf("Failed to get a file handle to our target data file! %s", err)
	}

	// Create an I/O writer buffer to pass to the snapshot when we ask it to serialize itself.
	targetFileWriter := bufio.NewWriter(targetFileHandle)

	defer func() {
		if err := targetFileWriter.Flush(); err != nil {
			log.Fatalf("Failed to flush the writer for '%s'!: %s", targetDataFile, err)
		}

		if err := targetFileHandle.Close(); err != nil {
			log.Fatalf("Failed to close the file handle for '%s'! %s", targetDataFile, err)
		}
	}()

	// Tell the snapshot to serialize/write itself out.
	if err := value.WriteTo(targetFileWriter); err != nil {
		return fmt.Errorf("Failed to read from snapshot during persisting to disk! %s", err)
	}

	return nil
}

func (me *FileStore) clean() error {
	return nil
}

func createDirectoryIfNotExists(path string) error {
	// Check if the directory even exist.
	if !doesDirectoryExist(path) {
		// It doesn't exist, so let's create it.
		if err := os.MkdirAll(path, 0775); err != nil {
			return fmt.Errorf("Unable to create folder! Path: %s, Error: %s", path, err)
		}
	}

	// If we're here, our folder exited or was created without error.
	return nil
}

func doesDirectoryExist(path string) bool {
	if stat, _ := os.Stat(path); stat != nil && stat.IsDir() {
		return true
	}

	return false
}

func doesFileExist(path string) bool {
	if stat, _ := os.Stat(path); stat != nil && !stat.IsDir() {
		return true
	}

	return false
}

func getFileWithoutExtension(filename string) string {
	baseFile := filepath.Base(filename)
	return strings.Replace(baseFile, filepath.Ext(baseFile), "", -1)
}

func hydrateSnapshotFromFile(path string) (*Snapshot, error) {
	// Let's open the data file and give the snapshot a reader to deserialize itself from.
	snapshotFileHandle, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("Failed to get a file handle to our latest data file! %s", err)
	}

	// Create an I/O reader buffer to pass to the snapshot when we ask it to unserialize itself.
	snapshotFileReader := bufio.NewReader(snapshotFileHandle)

	defer func() {
		if err := snapshotFileHandle.Close(); err != nil {
			log.Fatalf("Failed to close the file handle for '%s'! %s", path, err)
		}
	}()

	// Ask for a snapshot back.
	snapshot, err := NewSnapshotFromReader(snapshotFileReader)
	if err != nil {
		return nil, err
	}

	return snapshot, nil
}
