package cadastre

import "fmt"
import "time"
import "github.com/kylelemons/go-gypsy/yaml"

// Provides the basic information needed to run Cadastre: what servers to check,
// how often to check them, and where to put the data.
type Configuration struct {
    // The list of MySQL hosts to check. A host is defined by a unique, internal name,
    // a display name (something more suitable than the internal name or hostname),
    // an optional category to allow for logical UI grouping and a DSN used to
    // figure out how to connect to the MySQL host.
    Servers []Server

    // The interval to check each server.
    FetchInterval time.Duration

    // The storage engine used to persist the host information to.
    Storage DataStore

    // Where to load templates from.
    TemplateDirectory string

    // Where to serve static assets from.
    StaticAssetDirectory string

    // Whether or not we're in debug mode.
    DebugMode bool
}

func LoadConfigurationFromFile(configurationFile string) (*Configuration, error) {
    config := &Configuration{}

    // Try to load the file we got passed to us.
    yamlConfig, err := yaml.ReadFile(configurationFile)
    if err != nil {
        return nil, fmt.Errorf("Caught an error while trying to load the configuration! %s", err)
    }

    // Get the fetch interval.
    fetchInterval, err := yamlConfig.Get("fetchInterval")
    if err != nil {
        return nil, fmt.Errorf("You must specify a fetch interval!")
    }

    parsedFetchInterval, err := time.ParseDuration(fetchInterval)
    if err != nil {
        return nil, fmt.Errorf("Fetch interval must be a valid duration string! i.e. 15s, 60m, 1h, 24h")
    }

    if int64(parsedFetchInterval) < 0 {
        return nil, fmt.Errorf("The specified fetch interval must be non-zero!")
    }

    config.FetchInterval = parsedFetchInterval

    // Get the template directory and the static asset directory.
    templateDirectory, err := yamlConfig.Get("templateDirectory")
    if err != nil || templateDirectory == "" {
        return nil, fmt.Errorf("Template directory must be specified!")
    }

    config.TemplateDirectory = templateDirectory

    staticAssetDirectory, err := yamlConfig.Get("staticAssetDirectory")
    if err != nil || staticAssetDirectory == "" {
        return nil, fmt.Errorf("Static asset directory must be specified!")
    }

    config.StaticAssetDirectory = staticAssetDirectory

    // Set up the specified storage engine.
    storageEngine, err := yamlConfig.Get("storageEngine.name")
    if err != nil {
        return nil, fmt.Errorf("Storage engine must be specified!")
    }

    switch storageEngine {
    case "file":
        fileStore := &FileStore{}

        dataDirectory, err := yamlConfig.Get("storageEngine.dataDirectory")
        if err != nil {
            return nil, fmt.Errorf("You must specify a data directory when using the file storage engine!")
        }

        fileStore.DataDirectory = dataDirectory

        retentionPeriod, err := yamlConfig.Get("storageEngine.retentionPeriod")
        if err != nil {
            return nil, fmt.Errorf("You must specify a retention period when using the file storage engine!")
        }

        parsedRetentionPeriod, err := time.ParseDuration(retentionPeriod)
        if err != nil {
            return nil, fmt.Errorf("Retention period must be a valid duration string! i.e. 15s, 60m, 1h, 24h")
        }

        if int64(parsedRetentionPeriod) < 0 {
            return nil, fmt.Errorf("The specified retention period must be non-zero!")
        }

        fileStore.RetentionPeriod = parsedRetentionPeriod

        config.Storage = fileStore
    default:
        return nil, fmt.Errorf("'%s' is not a recognized storage engine!", storageEngine)
    }

    // Now parse out the servers we want to monitor.
    serverCount, err := yamlConfig.Count("servers")
    if err != nil || serverCount < 1 {
        return nil, fmt.Errorf("You must specify MySQL servers to monitor!")
    }

    servers := []Server{}
    for i := 0; i < serverCount; i++ {
        // Get the internal name for this server.
        internalName, err := yamlConfig.Get(fmt.Sprintf("servers[%d].internalName", i))
        if err != nil {
            return nil, fmt.Errorf("You must specify an internal name for all monitored servers!")
        }

        // Get the display name for this server.
        displayName, err := yamlConfig.Get(fmt.Sprintf("servers[%d].displayName", i))
        if err != nil {
            return nil, fmt.Errorf("You must specify a display name for all monitored servers!")
        }

        // Get the category for this server.
        category, _ := yamlConfig.Get(fmt.Sprintf("servers[%d].category", i))

        // Get the DSN for this server.
        dataSourceName, err := yamlConfig.Get(fmt.Sprintf("servers[%d].dataSourceName", i))
        if err != nil {
            return nil, fmt.Errorf("You must specify a data source name for all monitored servers!")
        }

        // Make sure the internal name and display name are unique.
        for _, existingServer := range servers {
            if existingServer.InternalName == internalName {
                return nil, fmt.Errorf("All monitored servers must have a unique, internal name! Duplicate name: %s", internalName)
            }

            if existingServer.DisplayName == displayName {
                return nil, fmt.Errorf("All monitored servers must have a unique, display name! Duplicate name: %s", internalName)
            }
        }

        server := Server{
            InternalName: internalName,
            DisplayName: displayName,
            Category: category,
            DataSourceName: dataSourceName,
        }

        servers = append(servers, server)
    }

    config.Servers = servers

    // Now that we're done parsing the configuration, make sure the configuration is ready to
    // go and initialize anything we need to initialize, etc.
    config.Storage.Initialize()

    return config, nil
}
