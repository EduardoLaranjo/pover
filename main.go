// Command pover is a small server for the Apprise API's stateless POST /notify,
// enough for clients such as Doco-CD. It sends to Pushover (pover:// URLs).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const defaultListen = ":8000"

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	listen := os.Getenv("POVER_LISTEN")
	if listen == "" {
		listen = defaultListen
	}

	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck(listen))
	}

	if err := serve(listen); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func serve(listen string) error {
	srv := &http.Server{
		Addr:              listen,
		Handler:           newMux(newPushover(pushoverAPI)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		// Senders answer only after delivering, which can take a few retries' worth.
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errc := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", listen)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdown); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}

// healthcheck asks the local server for /status; the image has no shell or curl.
func healthcheck(listen string) int {
	client := &http.Client{Timeout: 3 * time.Second}

	_, port, err := net.SplitHostPort(listen)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	resp, err := client.Get("http://" + net.JoinHostPort("127.0.0.1", port) + "/status")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "status:", resp.Status)
		return 1
	}

	return 0
}
