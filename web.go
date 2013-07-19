package cadastre

import "fmt"
import "log"
import "os"
import "sync/atomic"
import "time"
import "bytes"
import "path/filepath"
import "html/template"
import "encoding/base64"
import "net/http"
import "github.com/tobz/fsnotify"
import "github.com/tobz/go-cache"

type WebUI struct {
	Configuration *Configuration

	server *http.Server

	templateCache   *cache.Cache
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
	me.templateCache = cache.New((time.Minute * 30), (time.Second * 30))
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
				me.templateCache.Flush()
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
	requestMultiplexer := http.NewServeMux()

	// Create our server instance using our request multiplexer.
	me.server = &http.Server{
		Addr:    "0.0.0.0:8080",
		Handler: requestMultiplexer,
	}

	log.Printf("Configuring request router...")

	// Define our routes.
	requestMultiplexer.Handle("/favicon.ico", CadastreHandler(me, me.serveFavicon))
	requestMultiplexer.Handle("/", CadastreHandler(me, me.serveIndex))

	absStaticAssetPath, err := filepath.Abs(me.Configuration.StaticAssetDirectory)
	if err != nil {
		return fmt.Errorf("Caught an error trying to get the absolute path to the static asset directory! %s", err)
	}

	requestMultiplexer.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(absStaticAssetPath))))

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

	encodedFavicon := "iVBORw0KGgoAAAANSUhEUgAAABAAAAAQEAYAAABPYyMiAAAABmJLR0T///////8JWPfcAAAACXBIWXMAAABIAAAASABGyWs+AAAAF0lEQVRIx2NgGAWjYBSMglEwCkbBSAcACBAAAeaR9cIAAAAASUVORK5CYII="
	imageData, err := base64.StdEncoding.DecodeString(encodedFavicon)
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

func incrementCounter(addr *uint64) {
	atomic.AddUint64(addr, 1)
}

func (me *WebUI) renderTemplate(response http.ResponseWriter, templatePath string, content map[string]interface{}) error {
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
	errorOutput := ""
	response.Write([]byte(errorOutput))
}

type CadastreWebHandler struct {
	Handler func(http.ResponseWriter, *http.Request) error
	Server  *WebUI
}

func (me *CadastreWebHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	err := me.Handler(response, request)
	if err != nil {
		incrementCounter(&(me.Server.RequestErrors))
	}

	incrementCounter(&(me.Server.RequestsServed))
}

func CadastreHandler(server *WebUI, handlerFunc func(http.ResponseWriter, *http.Request) error) *CadastreWebHandler {
	return &CadastreWebHandler{Handler: handlerFunc, Server: server}
}
