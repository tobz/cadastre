package cadastre

import "os"
import "time"
import "fmt"
import "log"
import "path/filepath"
import "bufio"
import "regexp"
import "strings"

type FileStore struct {
    DataDirectory string
    RetentionPeriod time.Duration
}

func (me *FileStore) Initialize() (error) {
    return nil
}

func (me *FileStore) RetrieveLatest(identifier string) (*Snapshot, error) {
    // Compile a regexp right quick so we can quickly match potential directories.
    dateRegexp, err := regexp.Compile("^\\d{4}-\\d{2}-\\d{2}$")
    if err != nil {
        return nil, fmt.Errorf("Failed to compile regular expression for matching data directories! %s", err)
    }

    datetimeRegexp, err := regexp.Compile("^\\d{4}-\\d{2}-\\d{2}-\\d{2}-\\d{2}-\\d{2}$")
    if err != nil {
        return nil, fmt.Errorf("Failed to compile regular expression for matching data files! %s", err)
    }

    // Make sure our data directory is a qualified, absolute path.
    absDataDirectory, err := filepath.Abs(me.DataDirectory)
    if err != nil {
        return nil, fmt.Errorf("Error normalizing the data directory path! %s", err)
    }

    // First, we need to find the top level time bucket so we can delve in.
    var latestTopLevelDirectory string
    filepath.Walk(absDataDirectory, func(path string, info os.FileInfo, _ error) error {
        // See if this is a directory and if it has a subdirectory for our identifier.
        if !info.IsDir() || !doesDirectoryExist(filepath.Join(path, identifier)) {
            return nil
        }

        baseDirectory := filepath.Dir(path)

        // Make sure this directory matches the YYYY-MM-DD format.
        if !dateRegexp.MatchString(baseDirectory) {
            return nil
        }

        baseDirectoryTime, err := time.Parse("2006-01-02", baseDirectory)
        if err == nil {
            // The directory matched, so now see if we need to compare to a previously-discovered directory.
            if latestTopLevelDirectory == "" {
                // This is the first directory we've seen, so it is now our latest.
                latestTopLevelDirectory = baseDirectory
            } else {
                // See if the value we just got is newer than the latest value we have.
                latestDirectoryTime, err := time.Parse("2006-01-02", latestTopLevelDirectory)
                if err != nil {
                    if baseDirectoryTime.After(latestDirectoryTime) {
                        latestTopLevelDirectory = baseDirectory
                    }
                } else {
                    // We had an error, so this is our new latest directory.
                    latestTopLevelDirectory = baseDirectory
                }
            }
        }

        return nil
    })

    // See if we actually found a viable top-level directory in our search.
    if latestTopLevelDirectory == "" {
        // We didn't find anything.... that's weird.  Maybe we only just started?  In any case, it could
        // be a case of legitimately having no data, so just return nil.
        return nil, fmt.Errorf("No data available for the given time period.")
    }

    // Now let's find the newest file for our identifier under the latest top-level directory.
    latestIdentifierDirectory := filepath.Join(absDataDirectory, latestTopLevelDirectory, identifier)

    var latestDataFile string
    filepath.Walk(latestIdentifierDirectory, func(path string, info os.FileInfo, _ error) error {
        // We're only looking for .spl files here, so skip directories and non-spl files.
        if info.IsDir() || filepath.Ext(path) != ".spl" {
            return nil
        }

        baseFile := filepath.Base(path)
        baseFileNoExt := getFileWithoutExtension(baseFile)

        // Make sure this file matches our file naming format of YYYY-MM-DD-hh-mm-ss.
        if !datetimeRegexp.MatchString(baseFileNoExt) {
            return nil
        }

        // Parse our filename into a datetime so we can figure out if we have the latest file.
        baseFileTime, err := time.Parse("2006-01-02-15-04-05", baseFileNoExt)
        if err != nil {
            return nil
        }

        // Compare the current file with the latest file we know about.
        if latestDataFile == "" {
            // Looks like this is the first file we've seen.  It's now our latest.
            latestDataFile = baseFile
        } else {
            latestDataFileTime, err := time.Parse("2006-01-02-15-04-05", getFileWithoutExtension(latestDataFile))
            if err != nil {
                // Looks like we had an error parsing the file for the latest file, so the current is now our latest.
                latestDataFile = baseFile
            } else {
                // See if our current file is newer than the latest we know about.
                if baseFileTime.After(latestDataFileTime) {
                    // It's newer, so it is now our latest.
                    latestDataFile = baseFile
                }
            }
        }

        return nil
    })

    // Make sure we found something in that last search.
    if latestDataFile == "" {
        return nil, fmt.Errorf("No data available for the given time period.")
    }

    // We have our latest data file now, so let's open our data file and give the snapshot a reader to deserialize itself from.
    latestDataFileHandle, err := os.Open(latestDataFile);
    if err != nil {
        return nil, fmt.Errorf("Failed to get a file handle to our latest data file! %s", err)
    }

    // Create an I/O reader buffer to pass to the snapshot when we ask it to unserialize itself.
    latestDataFileReader := bufio.NewReader(latestDataFileHandle)

    defer func() {
        if err := latestDataFileHandle.Close(); err != nil {
            log.Fatalf("Failed to close the file handle for '%s'! %s", latestDataFile, err)
        }
    }()

    // Ask for a snapshot back.
    latestSnapshot, err := NewSnapshotFromReader(latestDataFileReader)
    if err != nil {
        return nil, err
    }

    return latestSnapshot, nil
}

func (me *FileStore) RetrieveRange(identifier string, start time.Time, end time.Time) (map[time.Time] *Snapshot, error) {
    return nil, nil
}

func (me *FileStore) Persist(identifier string, timestamp time.Time, value *Snapshot) error {
    // Make sure the folder exists for our identifier, and within that, today's date.  Create either if they don't exist.
    topLevelTime := timestamp.Format("2006-01-02")
    lowLevelTime := timestamp.Format("2006-01-02-15-04-05")

    targetDirectory, err := filepath.Abs(filepath.Join(me.DataDirectory, topLevelTime, identifier));
    if err != nil {
        return fmt.Errorf("Encountered an error trying to figure out the data directory! %s", err)
    }

    if err := createDirectoryIfNotExists(targetDirectory); err != nil {
        return fmt.Errorf("Failed to create the directory to store a process list snapshot! Identifier: %s, Error: %s", identifier, err)
    }

    targetDataFile := filepath.Join(targetDirectory, lowLevelTime + ".spl")

    // We have our target directory now, so let's create/open our data file and give the snapshot a writer to serialize itself to.
    targetFileHandle, err := os.OpenFile(targetDataFile, os.O_RDWR | os.O_CREATE | os.O_TRUNC, 0774);
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

func (me *FileStore) clean() (error) {
    return nil
}

func createDirectoryIfNotExists(path string) (error) {
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

func doesDirectoryExist(path string) (bool) {
    if stat, _ := os.Stat(path); stat != nil && stat.IsDir() {
        return true
    }

    return false
}

func getFileWithoutExtension(file string) (string) {
    baseFile := filepath.Base(file)
    return strings.Replace(baseFile, filepath.Ext(baseFile), "", -1)
}
