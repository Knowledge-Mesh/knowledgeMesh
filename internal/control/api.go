package control

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/matchmaker"
	"github.com/spf13/cobra"
)

// NewAPICommand runs the HTTP control pane API (buyer registration in PostgreSQL).
func NewAPICommand() *cobra.Command {
	var (
		httpAddr  string
		jwtSecret string
	)

	cmd := &cobra.Command{
		Use:   "api",
		Short: "Run control pane HTTP API (buyers + sellers; requires PostgreSQL)",
		Long: `Listens for HTTP requests. Buyer and seller accounts and seller models are stored in PostgreSQL (DATABASE_URL).

On startup, pending SQL migrations are applied (embedded from internal/control/migrations via golang-migrate; version table schema_migrations).

Set CONTROL_JWT_SECRET in production (or pass --jwt-secret).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dsn := strings.TrimSpace(os.Getenv("DATABASE_URL"))
			if dsn == "" {
				return errors.New("DATABASE_URL is required for the control API (PostgreSQL connection string)")
			}
			if jwtSecret == "" {
				jwtSecret = os.Getenv("CONTROL_JWT_SECRET")
			}
			if jwtSecret == "" {
				jwtSecret = "dev-change-me"
				log.Printf("warning: using default JWT secret; set CONTROL_JWT_SECRET or --jwt-secret for production")
			}

			ctx := context.Background()
			if err := RunMigrations(ctx, dsn); err != nil {
				return err
			}
			store, err := NewPostgresStore(ctx, dsn)
			if err != nil {
				return err
			}

			srv := &HTTPServer{
				Store:   store,
				Secret:  []byte(jwtSecret),
				Matcher: matchmaker.NewService(),
			}

			log.Printf("control pane API listening on %s", httpAddr)
			return http.ListenAndServe(httpAddr, srv.Handler())
		},
	}

	cmd.Flags().StringVar(&httpAddr, "http-addr", ":8090", "HTTP listen address")
	cmd.Flags().StringVar(&jwtSecret, "jwt-secret", "", "HMAC secret for buyer JWTs (default: env CONTROL_JWT_SECRET)")
	return cmd
}
