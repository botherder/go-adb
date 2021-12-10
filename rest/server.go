package rest

import (
	"fmt"
	"net/http"
	pprof "net/http/pprof"
	"time"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

//limitNumClients uses a go channel to rate limit a handler.
// It is a golang buffered channel so you can put maxClients empty structs
//into the channel without blocking. The maxClients+1 invocation will block
//until another handler finishes and removes one empty struct from the channel.
func limitNumClients(f http.HandlerFunc, maxClients int) http.HandlerFunc {
	sema := make(chan struct{}, maxClients)

	return func(w http.ResponseWriter, req *http.Request) {
		sema <- struct{}{}
		defer func() { <-sema }()
		f(w, req)
	}
}

func notFoundHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverError("not found", http.StatusNotFound, w)
	})
}

func methodNotAllowedHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverError("method not allowed", http.StatusMethodNotAllowed, w)
	})
}

//CreateRouter creates a new router and exposes the workspace to
//the http handlers.
func CreateRouter(s BridgeStatusReporter) *mux.Router {
	r := mux.NewRouter()
	r.MethodNotAllowedHandler = methodNotAllowedHandler()
	r.NotFoundHandler = notFoundHandler()

	r.HandleFunc("/devices", limitNumClients(HealthHandler(s), 1)).Methods("GET")
	r.HandleFunc("/devices/{serial}/reset", limitNumClients(DeviceResetHandler, 1)).Methods("POST")
	r.HandleFunc("/devices/{vid}/{pid}/reset", limitNumClients(DeviceResetVidPidHandler, 1)).Methods("POST")
	attachProfiler(r)
	return r
}

//attachProfiler enables pprof rest interfaces on the gorilla mux
func attachProfiler(router *mux.Router) {
	router.HandleFunc("/debug/pprof/", pprof.Index)
	router.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	router.HandleFunc("/debug/pprof/profile", pprof.Profile)
	router.HandleFunc("/debug/pprof/symbol", pprof.Symbol)

	// Manually add support for paths linked to by index page at /debug/pprof/
	router.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	router.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	router.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
	router.Handle("/debug/pprof/block", pprof.Handler("block"))
	router.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
	router.Handle("/debug/pprof/trace", pprof.Handler("trace"))
	router.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))
}

//CreateHTTPServer creates a *http.Server with routes added by the Createrouter func.
//It also configures timeouts, which is important because default timeouts are set to 0
//which can cause tcp connections being open indefinitely.
func CreateHTTPServer(address string, s BridgeStatusReporter) *http.Server {
	srv := &http.Server{
		Handler:      CreateRouter(s),
		Addr:         address,
		WriteTimeout: 60 * time.Second,
		ReadTimeout:  60 * time.Second,
	}

	return srv
}

func StartHttpServer(restInterfacePort int, s BridgeStatusReporter) *http.Server {
	srv := CreateHTTPServer(fmt.Sprintf("0.0.0.0:%d", restInterfacePort), s)

	go func() {
		err := srv.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			log.WithFields(log.Fields{"err": err}).Fatal("wrapper http server failed")
		}
	}()
	return srv
}
