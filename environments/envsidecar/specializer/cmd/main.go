package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/fission/fission"
	"github.com/fission/fission/environments/envsidecar"
)

func dumpStackTrace() {
	debug.PrintStack()
}

// Usage: specializer <shared volume path>
func main() {
	// register signal handler for dumping stack trace.
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM)
	go func() {
		<-c
		log.Println("Received SIGTERM : Dumping stack trace")
		dumpStackTrace()
		os.Exit(1)
	}()

	flag.Usage = specializerUsage
	specializeOnStart := flag.Bool("specialize-on-startup", false, "Flag to activate specialize process at pod starup")
	specializePayload := flag.String("-specialize-request", "", "JSON payload for specialize request")
	secretDir := flag.String("secret-dir", "", "Path to shared secrets directory")
	configDir := flag.String("cfgmap-dir", "", "Path to shared configmap directory")

	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(1)
	}

	dir := flag.Arg(0)
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(dir, os.ModeDir|0700)
			if err != nil {
				log.Fatalf("Error creating directory: %v", err)
			}
		}
	}

	f, err := envsidecar.MakeEnvSidecar(dir, *secretDir, *configDir)
	if err != nil {
		log.Fatalf("Error making specializer: %v", err)
	}

	var success bool

	if *specializeOnStart {
		var specializeReq fission.FunctionSpecializeRequest

		err := json.Unmarshal([]byte(*specializePayload), &specializeReq)
		if err != nil {
			log.Fatalf("Error decoding specialize request: %v", err)
		}

		err = f.SpecializePod(specializeReq.FetchReq, specializeReq.LoadReq)
		if err != nil {
			log.Fatalf("Error specialing function poadt: %v", err)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/fetch", f.FetchHandler)
	mux.HandleFunc("/specialize", f.SpecializeHandler)
	mux.HandleFunc("/upload", f.UploadHandler)
	mux.HandleFunc("/version", f.VersionHandler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		var statusCode int

		if *specializeOnStart {
			if success {
				statusCode = http.StatusOK
			} else {
				statusCode = http.StatusInternalServerError
			}
		}

		w.WriteHeader(statusCode)
	})

	log.Println("Fetcher ready to receive requests")
	http.ListenAndServe(":8000", mux)
}

func specializerUsage() {
	fmt.Printf("Usage: specializer [-specialize-on-startup] [-fetch-request <json>] [-load-request <json>] [-secret-dir <string>] [-cfgmap-dir <string>] <shared volume path> \n")
}