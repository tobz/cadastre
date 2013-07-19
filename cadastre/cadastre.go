package main

import "log"
import "flag"
import "os"
import "os/signal"
import "time"
import "cadastre"

var (
	configurationFile = flag.String("config", "", "the path to the Cadastre configuration file to load")
	debugMode         = flag.Bool("debug", false, "whether or not to run the daemon in debug mode")
)

func init() {
	flag.Parse()

	if *configurationFile == "" {
		log.Fatal("You must specify the configuration file to load!")
	}
}

func main() {
	// Try and load our configuration.
	log.Printf("Loading configuration...")

	config, err := cadastre.LoadConfigurationFromFile(*configurationFile)
	if err != nil {
		log.Fatalf("Couldn't load configuration file! %s", err)
	}

	config.DebugMode = *debugMode

	// Start up our background fetcher.
	log.Printf("Starting up background fetcher...")

	fetcher := &cadastre.Fetcher{Configuration: config}

	err = fetcher.Start()
	if err != nil {
		log.Fatalf("Couldn't start background fetcher! %s", err)
	}

	// Spin up our web server.
	log.Printf("Starting up web server...")

	webServer := &cadastre.WebUI{Configuration: config}
	err = webServer.StartListening()
	if err != nil {
		log.Fatalf("Couldn't start web server! %s", err)
	}

	log.Printf("Cadastre is now ready to serve requests.")

	// Wait until we're told to exit and spin until that happens.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)

	debugTick := time.Tick(time.Second * 5)

	for {
		select {
		case <-sig:
			log.Printf("Shutting down fetcher and exiting...")

			fetcher.Stop()
			os.Exit(0)
		case <-debugTick:
			if *debugMode {
				cacheHitRate := 0.0
				if webServer.CacheRequests != 0 {
					cacheHitRate = ((float64(webServer.CacheHits) / float64(webServer.CacheRequests)) * float64(100))
				}
				log.Printf("web stats: cache (requests / hit rate): %d / %.2f%s", webServer.CacheRequests, cacheHitRate, "%")
			}
		}
	}
}
