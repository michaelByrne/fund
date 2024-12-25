package root

import (
	"context"
	"errors"
	"fmt"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/spf13/cobra"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func RootCmd(ctx context.Context, server *http.Server, ns *server.Server) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fund",
		Short: "bco mutual aid",
		RunE: func(cmd *cobra.Command, args []string) error {
			serverCtx, serverStopCtx := context.WithCancel(ctx)

			sig := make(chan os.Signal, 1)
			signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

			go func() {
				<-sig

				shutdownCtx, _ := context.WithTimeout(serverCtx, 30*time.Second)

				go func() {
					<-shutdownCtx.Done()
					if errors.Is(shutdownCtx.Err(), context.DeadlineExceeded) {
						log.Fatal("graceful shutdown timed out.. forcing exit.")
					}

					ns.Shutdown()
				}()

				err := server.Shutdown(shutdownCtx)
				if err != nil {
					log.Fatal(err)
				}

				ns.WaitForShutdown()

				serverStopCtx()
			}()

			log.Println("** starting bco fund on port 8080 **")
			err := server.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				return fmt.Errorf("server failed with error: %w", err)
			}

			<-serverCtx.Done()

			return nil
		},
	}

	return cmd
}
