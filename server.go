package cadastre

// Defines a MySQL server that can be checked.
type Server struct {
	// The category this server belongs to.  This is used for logical grouping with the UI.
	Category string

	// The name of the server to show in the UI.
	DisplayName string

	// The internal name of the server, which should be unique.  This is used when
	// persisting and retrieiving snapshots for the host from the storage engine.
	InternalName string

	// The canonical data source name for the given server.  This includes hostname,
	// port, username, password, SSL options, etc.
	DataSourceName string
}
