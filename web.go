package cadastre

import "fmt"
import "log"
import "os"
import "io"
import "strings"
import "strconv"
import "sync/atomic"
import "time"
import "bytes"
import "path/filepath"
import "html/template"
import "encoding/base64"
import "encoding/json"
import "compress/gzip"
import "net/http"
import "github.com/howeyc/fsnotify"
import "github.com/tobz/gocache"
import "github.com/gorilla/mux"

type WebUI struct {
	Configuration *Configuration

	server *http.Server

	templateCache   *gocache.Cache
	templateWatcher *fsnotify.Watcher

	RequestsServed uint64
	RequestErrors  uint64
	CacheRequests  uint64
	CacheHits      uint64
}

func (me *WebUI) StartListening() error {
	var err error

	// Initiailize our statistics.
	me.RequestsServed = 0
	me.RequestErrors = 0
	me.CacheRequests = 0
	me.CacheHits = 0

	log.Printf("Instantiating template cache...")

	// Set up our template cache and template watcher.
	me.templateCache = gocache.New((time.Minute * 30), (time.Second * 30))
	me.templateWatcher, err = fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("Unable to instantiate template watcher! %s", err)
	}

	go func() {
		for {
			select {
			// Something happened in our template directory.  Invalidate the cache.
			case ev := <-me.templateWatcher.Event:
				log.Printf("Detected change to template file '%s'.  Flushing template cache.", ev.Name)
				me.templateCache.Clear()
			}
		}
	}()

	absTemplateDirectory, err := filepath.Abs(me.Configuration.TemplateDirectory)
	if err != nil {
		return fmt.Errorf("Caught an error trying to get the absolute path to the template directory! %s", err)
	}

	// Watch our template directory for changes so that we invalidate the cache  Only watch for modifications and deletes.
	me.templateWatcher.WatchFlags(absTemplateDirectory, fsnotify.FSN_MODIFY)

	log.Printf("Creating HTTP server...")

	// Set up our request multiplexer.
	requestMultiplexer := mux.NewRouter()

	// Create our server instance using our request multiplexer.
	me.server = &http.Server{
		Addr:        me.Configuration.ListenAddress,
		Handler:     requestMultiplexer,
		ReadTimeout: time.Second * 30,
	}

	log.Printf("Configuring request router...")

	// Define our routes.
	requestMultiplexer.Handle("/favicon.ico", CadastreHandler(me, me.serveFavicon))
	requestMultiplexer.Handle("/", CadastreHandler(me, me.serveIndex))
	requestMultiplexer.Handle("/_getServerGroups", CadastreHandler(me, me.serveServerGroups))
	requestMultiplexer.Handle("/_getCurrentSnapshot/{serverName}", CadastreHandler(me, me.serveCurrentSnapshot))
	requestMultiplexer.Handle("/_getPreviousSnapshot/{serverName}/{timestamp}", CadastreHandler(me, me.servePreviousSnapshot))

	absStaticAssetPath, err := filepath.Abs(me.Configuration.StaticAssetDirectory)
	if err != nil {
		return fmt.Errorf("Caught an error trying to get the absolute path to the static asset directory! %s", err)
	}

	requestMultiplexer.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir(absStaticAssetPath))))

	// Start listening!
	go func() {
		err := me.server.ListenAndServe()
		if err != nil {
			log.Printf("Caught an error while serving requests! %s", err)
		}
	}()

	return nil
}

func (me *WebUI) loadTemplate(templatePath string) (*template.Template, error) {
	incrementCounter(&(me.CacheRequests))

	// Try the cache first.
	cachedTemplate, found := me.templateCache.Get(templatePath)
	if found {
		incrementCounter(&(me.CacheHits))

		return cachedTemplate.(*template.Template), nil
	}

	// Not in the cache, so we need to go to disk for it.  Build our path.
	absTemplatePath, err := filepath.Abs(filepath.Join(me.Configuration.TemplateDirectory, templatePath))
	if err != nil {
		return nil, fmt.Errorf("Error building the proper path to load the template file! %s", err)
	}

	// Open the template.
	fileHandle, err := os.Open(absTemplatePath)
	if err != nil {
		return nil, fmt.Errorf("Error loading template file! %s", err)
	}
	defer fileHandle.Close()

	// Read out the template.
	templateBuffer := bytes.NewBuffer([]byte{})
	_, err = templateBuffer.ReadFrom(fileHandle)
	if err != nil {
		return nil, fmt.Errorf("Error reading the template file! %s", err)
	}

	// Create our actual template.
	newTemplate := template.New(templatePath)
	newTemplate, err = newTemplate.Parse(templateBuffer.String())
	if err != nil {
		return nil, fmt.Errorf("Error creating template object! %s", err)
	}

	// Put this in the cache.
	me.templateCache.Set(templatePath, newTemplate, -1)

	return newTemplate, nil
}

func (me *WebUI) serveFavicon(response http.ResponseWriter, request *http.Request) error {
	response.Header().Add("Content-Type", "image/x-icon")

	imageData, err := base64.StdEncoding.DecodeString(CadastreEncodedFavicon)
	if err != nil {
		serveErrorPage(response)
		return fmt.Errorf("request error: couldn't decode favicon data")
	}

	response.Write(imageData)

	return nil
}

func (me *WebUI) serveIndex(response http.ResponseWriter, request *http.Request) error {
	return me.renderTemplate(response, "index.goml", nil)
}

func (me *WebUI) serveServerGroups(response http.ResponseWriter, request *http.Request) error {
	// Wrap our result.
	result := make(map[string]interface{}, 0)
	result["groups"] = me.Configuration.ServerGroups

	return me.renderJson(response, result)
}

func incrementCounter(addr *uint64) {
	atomic.AddUint64(addr, 1)
}

func (me *WebUI) serveCurrentSnapshot(response http.ResponseWriter, request *http.Request) error {
	// Get the specified server name.
	requestVars := mux.Vars(request)
	internalName := requestVars["serverName"]

	// Try and find the given server in our list of servers.
	for _, server := range me.Configuration.Servers {
		if server.InternalName == internalName {
			// Found our server, so try and take a snapshot.
			newSnapshot := &Snapshot{}
			err := newSnapshot.TakeSnapshot(server)
			if err != nil {
				return me.renderJsonError(response, fmt.Sprintf("Failed to take processlist snapshot for <b>%s</b>! %s", internalName, err.Error()))
			}

			return me.renderJson(response, newSnapshot)
		}
	}

	// We never found the server, so inform the UI.  This is definitely a bug.
	return me.renderJsonError(response, fmt.Sprintf("Failed to find server <b>%s</b> in the configured list of servers!", internalName))
}

func (me *WebUI) servePreviousSnapshot(response http.ResponseWriter, request *http.Request) error {
	// Get the specified server name and timestamp.
	requestVars := mux.Vars(request)
	internalName := requestVars["serverName"]
	timestampRaw := requestVars["timestamp"]

	// Make sure the timestamp is valid and convert it to a Time object.
	timestamp, err := strconv.ParseInt(timestampRaw, 10, 64)
	if err != nil {
		return me.renderJsonError(response, fmt.Sprintf("The given timestamp is not a valid Unix timestamp! Timestamp given: %s", timestampRaw))
	}

	timestampTime := time.Unix(timestamp, 0)

	// Try and get a snapshot for the given time period.
	snapshot, err := me.Configuration.Storage.Retrieve(internalName, timestampTime)
	if err != nil {
		return me.renderJsonError(response, fmt.Sprintf("Failed to get a snapshot for the requested timestamp! %s", err.Error()))
	}

	// We got our snapshot, so return it.
	return me.renderJson(response, snapshot)
}

func (me *WebUI) renderJson(response http.ResponseWriter, data interface{}) error {
	response.Header().Set("Content-Type", "application/json")

	result := make(map[string]interface{}, 0)
	result["success"] = true
	result["payload"] = data

	jsonEncoder := json.NewEncoder(response)
	err := jsonEncoder.Encode(result)
	if err != nil {
		serveErrorPage(response)
		return fmt.Errorf("request error: failed to encode object to JSON! %s", err)
	}

	return nil
}

func (me *WebUI) renderJsonError(response http.ResponseWriter, errorMessage string) error {
	response.Header().Set("Content-Type", "application/json")

	result := make(map[string]interface{}, 0)
	result["success"] = false
	result["errorMessage"] = errorMessage

	jsonEncoder := json.NewEncoder(response)
	err := jsonEncoder.Encode(result)
	if err != nil {
		serveErrorPage(response)
		return fmt.Errorf("request error: failed to encode error object to JSON! %s", err)
	}

	return nil
}

func (me *WebUI) renderTemplate(response http.ResponseWriter, templatePath string, content map[string]interface{}) error {
	response.Header().Set("Content-Type", "text/html")

	// Load the specified template.
	templateData, err := me.loadTemplate(templatePath)
	if err != nil {
		serveErrorPage(response)
		return fmt.Errorf("request error: couldn't load template '%s': %s", templatePath, err)
	}

	// Generate our template output and write it to the response.
	err = templateData.Execute(response, content)
	if err != nil {
		serveErrorPage(response)
		return fmt.Errorf("request error: couldn't render template '%s': %s", templatePath, err)
	}

	return nil
}

func serveErrorPage(response http.ResponseWriter) {
	response.Header().Set("Content-Type", "text/plain")

	errorOutput := "Sorry, something went terribly wrong here. :("
	response.Write([]byte(errorOutput))
}

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (me *gzipResponseWriter) Write(b []byte) (int, error) {
	return me.Writer.Write(b)
}

type CadastreWebHandler struct {
	Handler func(http.ResponseWriter, *http.Request) error
	Server  *WebUI
}

func (me *CadastreWebHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	var err error

	// If the browser accepts gzip'd content, use that.
	if strings.Contains(request.Header.Get("Accept-Encoding"), "gzip") {
		response.Header().Set("Content-Encoding", "gzip")

		gzipWriter := gzip.NewWriter(response)
		defer gzipWriter.Close()

		gzipResponse := &gzipResponseWriter{Writer: gzipWriter, ResponseWriter: response}
		err = me.Handler(gzipResponse, request)
	} else {
		err = me.Handler(response, request)
	}

	if err != nil {
		incrementCounter(&(me.Server.RequestErrors))
	}

	incrementCounter(&(me.Server.RequestsServed))
}

func CadastreHandler(server *WebUI, handlerFunc func(http.ResponseWriter, *http.Request) error) *CadastreWebHandler {
	return &CadastreWebHandler{Handler: handlerFunc, Server: server}
}
