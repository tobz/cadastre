package cadastre

// Defines a MySQL server that can be checked.
type Server struct {
	// The name of the server to show in the UI.
	DisplayName string `json:"displayName"`

	// The internal name of the server, which should be unique.  This is used when
	// persisting and retrieiving snapshots for the host from the storage engine.
	InternalName string `json:"internalName"`

	// The name of the group this server belongs to.
	GroupName string `json:"-"`

	// The canonical data source name for the given server.  This includes hostname,
	// port, username, password, SSL options, etc.
	DataSourceName string `json:"-"`
}

// Defines a logical grouping of MySQL servers - testing vs production, etc
type ServerGroup struct {
	// The name of the group.
	GroupName string `json:"groupName"`

	// The servers that make up the group.
	Servers []Server `json:"servers"`
}
