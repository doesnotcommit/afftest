package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	urlpkg "net/url"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	shutdownTimeout = time.Second * 30
	listenPort      = 8080
	iLimit          = 100
	oLimit          = 4
	errLimit        = 1
	urlCountLimit   = 20
)

func main() {
	l := log.New(os.Stdout, "app", log.LstdFlags)
	srv := newServer(newMultiplexer(iLimit, oLimit, errLimit, &http.Transport{}, l))
	go func() {
		if err := srv.ListenAndServe(); errors.Is(err, http.ErrServerClosed) {
			l.Println(err)
		} else if err != nil {
			panic(err)
		}
	}()
	waitForShutdown(func() {
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		srv.Shutdown(ctx)
	})
}

func waitForShutdown(stopFuncs ...func()) {
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)
	<-done
	signal.Stop(done)
	for _, sf := range stopFuncs {
		sf()
	}
}

func newServer(handler http.Handler) *http.Server {
	mux := http.NewServeMux()
	srv := http.Server{
		Addr:    fmt.Sprintf(":%d", listenPort),
		Handler: mux,
	}
	mux.Handle("/", handler)
	return &srv
}

func newMultiplexer(iLimit, oLimit, errLimit int, rt http.RoundTripper, l *log.Logger) http.Handler {
	limitInbound := newSem(iLimit)
	cli := http.Client{
		Transport: rt,
		Timeout:   time.Second,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer limitInbound()()
		var urls []string
		if err := json.NewDecoder(r.Body).Decode(&urls); err != nil {
			l.Println(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if len(urls) > urlCountLimit {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		// minimal input validation
		for _, url := range urls {
			if _, err := urlpkg.Parse(url); err != nil {
				l.Println(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}
		incr := newSem(oLimit)
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		results := make([]string, len(urls))
		errs := make(chan error, errLimit)
		for i := range urls {
			decr := incr()
			go processRequest(ctx, i, urls[i], &cli, results, errs, cancel, decr)
		}
		// wait for workers to finish
		for i := 0; i < oLimit; i++ {
			incr()
		}
		select {
		case err := <-errs:
			l.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			respBody := map[string]string{
				"error": err.Error(),
			}
			if err := json.NewEncoder(w).Encode(respBody); err != nil {
				l.Println(err)
			}
			return
		default:
		}
		if err := json.NewEncoder(w).Encode(results); err != nil {
			l.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	})
}

func processRequest(ctx context.Context, i int, u string, cli *http.Client, results []string, errs chan<- error, cancel, decr func()) {
	defer decr()
	handleErr := func(err error) {
		cancel()
		errs <- fmt.Errorf("build request to %s: %w", u, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		handleErr(err)
		return
	}
	log.Printf("requesting %s", u)
	resp, err := cli.Do(req)
	if err != nil {
		handleErr(err)
		return
	}
	rawRespBody, err := io.ReadAll(resp.Body)
	if err != nil {
		handleErr(err)
		return
	}
	results[i] = string(rawRespBody)
}

func newSem(size int) func() func() {
	sem := make(chan struct{}, size)
	return func() func() {
		sem <- struct{}{}
		return func() {
			<-sem
		}
	}
}
