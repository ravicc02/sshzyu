package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/Wei-Shaw/sub2api/pkg/release"
	"github.com/ravicc02/sshzyu-updater/internal/updater"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "sshzy-updater:", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	if len(arguments) == 0 {
		return errors.New("expected serve, bundle, verify, bootstrap or recover")
	}
	flags := flag.NewFlagSet(arguments[0], flag.ContinueOnError)
	configPath := flags.String("config", "/etc/sshzy-updater/config.json", "Host configuration")
	manifestPath := flags.String("manifest", "", "Manifest file")
	signaturePath := flags.String("signature", "", "Signature file")
	publicKeyPath := flags.String("public-key", "", "Trusted Ed25519 public key")
	root := flags.String("repo-root", ".", "Committed repository checkout")
	output := flags.String("output", "", "New output directory")
	digest := flags.String("image-digest", "", "Registry image digest")
	key := flags.String("key-file", "", "Ed25519 PKCS8 PEM signing key")
	keyID := flags.String("key-id", "", "Trusted signing key ID")
	receipt := flags.String("verification-file", "", "Successful CI verification receipt")
	previous := flags.String("previous-manifest", "", "Verified prior manifest")
	confirmed := flags.Bool("confirm", false, "Explicitly authorize host state changes")
	operationID := flags.String("operation", "", "Interrupted operation ID")
	decision := flags.String("decision", "", "Recovery decision: complete or rollback")
	if err := flags.Parse(arguments[1:]); err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if arguments[0] == "bundle" {
		absolute, err := filepath.Abs(*root)
		if err != nil {
			return err
		}
		return updater.Bundle(ctx, updater.BundleOptions{Root: absolute, Output: *output, Digest: *digest, KeyFile: *key,
			KeyID: *keyID, VerificationFile: *receipt, PreviousManifest: *previous})
	}
	if arguments[0] == "verify" {
		data, err := os.ReadFile(*manifestPath)
		if err != nil {
			return errors.New("manifest unavailable")
		}
		signature, err := os.ReadFile(*signaturePath)
		if err != nil {
			return errors.New("signature unavailable")
		}
		key, err := updater.ReadPublicKey(*publicKeyPath)
		if err != nil {
			return errors.New("public key unavailable")
		}
		_, err = release.Verify(data, signature, key)
		return err
	}
	var config updater.Config
	data, err := os.ReadFile(*configPath)
	if err != nil || json.Unmarshal(data, &config) != nil {
		return errors.New("invalid host configuration")
	}
	source, err := updater.NewGitHubSource(config)
	if err != nil {
		return errors.New("release source trust configuration unavailable")
	}
	driver, err := updater.NewHostDriver(config, source)
	if err != nil {
		return err
	}
	if arguments[0] == "bootstrap" {
		if !*confirmed {
			return errors.New("bootstrap requires --confirm after manual installation")
		}
		return driver.Bootstrap(ctx, *manifestPath, *signaturePath)
	}
	manager, err := updater.NewManager(config, source, driver)
	if err != nil {
		return err
	}
	if arguments[0] == "recover" {
		if !*confirmed {
			return errors.New("recovery requires an explicit --confirm")
		}
		return manager.Recover(ctx, *operationID, *decision)
	}
	if arguments[0] != "serve" {
		return errors.New("unknown updater command")
	}
	return updater.Serve(ctx, config, manager, driver.ControlToken())
}
