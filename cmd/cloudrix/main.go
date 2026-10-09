// Command cloudrix runs the Cloudrix panel.
//
//	cloudrix                       start the panel
//	cloudrix admin create -u NAME  create a sudo admin (password from CLOUDRIX_ADMIN_PASSWORD or prompt)
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/api"
	"github.com/k9032431-cmd/cloudrix/internal/auth"
	"github.com/k9032431-cmd/cloudrix/internal/config"
	"github.com/k9032431-cmd/cloudrix/internal/jobs"
	"github.com/k9032431-cmd/cloudrix/internal/model"
	"github.com/k9032431-cmd/cloudrix/internal/store"
	"github.com/k9032431-cmd/cloudrix/web"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	cfg := config.Load()

	if len(os.Args) > 1 {
		if err := runCommand(cfg, os.Args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}
	if err := serve(cfg, log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func serve(cfg config.Config, log *slog.Logger) error {
	st, err := store.Open(cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer st.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := bootstrapAdmin(ctx, cfg, st, log); err != nil {
		return err
	}
	secret, err := jwtSecret(ctx, cfg, st)
	if err != nil {
		return err
	}

	srv := api.New(cfg, st, auth.NewIssuer(secret, cfg.TokenTTL), log, web.FS())
	runner := &jobs.Runner{Store: st, Interval: cfg.JobsInterval, Log: log}
	go runner.Run(ctx)

	httpSrv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errc := make(chan error, 1)
	go func() {
		log.Info("cloudrix panel listening", "addr", cfg.Listen, "version", api.Version)
		errc <- httpSrv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpSrv.Shutdown(shutdownCtx)
	}
	return nil
}

// bootstrapAdmin creates the first sudo admin from env when the panel is empty.
func bootstrapAdmin(ctx context.Context, cfg config.Config, st *store.Store, log *slog.Logger) error {
	n, err := st.CountAdmins(ctx)
	if err != nil || n > 0 {
		return err
	}
	if cfg.AdminUsername == "" || cfg.AdminPassword == "" {
		log.Warn("no admins exist: set CLOUDRIX_ADMIN_USERNAME/CLOUDRIX_ADMIN_PASSWORD or run `cloudrix admin create -u NAME`")
		return nil
	}
	if err := createAdmin(ctx, st, cfg.AdminUsername, cfg.AdminPassword); err != nil {
		return fmt.Errorf("bootstrap admin: %w", err)
	}
	log.Info("created sudo admin", "username", cfg.AdminUsername)
	return nil
}

func createAdmin(ctx context.Context, st *store.Store, username, password string) error {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	return st.CreateAdmin(ctx, &model.Admin{Username: username, PasswordHash: hash, Role: model.RoleSudo})
}

// jwtSecret uses the configured secret or a random one persisted in the DB,
// so sessions survive restarts without manual setup.
func jwtSecret(ctx context.Context, cfg config.Config, st *store.Store) (string, error) {
	if cfg.JWTSecret != "" {
		return cfg.JWTSecret, nil
	}
	secret, err := st.GetSetting(ctx, "jwt_secret")
	if err == nil {
		return secret, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return "", err
	}
	secret = auth.RandomToken(48)
	return secret, st.SetSetting(ctx, "jwt_secret", secret)
}

func runCommand(cfg config.Config, args []string) error {
	if len(args) >= 2 && args[0] == "admin" && args[1] == "create" {
		fs := flag.NewFlagSet("admin create", flag.ExitOnError)
		username := fs.String("u", "", "username")
		_ = fs.Parse(args[2:])
		if *username == "" {
			return errors.New("usage: cloudrix admin create -u NAME")
		}
		password := cfg.AdminPassword
		if password == "" {
			fmt.Fprint(os.Stderr, "password: ")
			line, err := bufio.NewReader(os.Stdin).ReadString('\n')
			if err != nil && line == "" {
				return err
			}
			password = strings.TrimSpace(line)
		}
		st, err := store.Open(cfg.DatabasePath)
		if err != nil {
			return err
		}
		defer st.Close()
		if err := createAdmin(context.Background(), st, *username, password); err != nil {
			return err
		}
		fmt.Println("sudo admin created:", *username)
		return nil
	}
	if args[0] == "version" {
		fmt.Println(api.Version)
		return nil
	}
	return fmt.Errorf("unknown command %q", strings.Join(args, " "))
}
