package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/AAQIB-LOCAL/Aaqib-OSS-Contributions/internal/audit"
)

func main() {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Fatal("GITHUB_TOKEN is required")
	}

	client := audit.NewGitHubClient(token)
	client.Logger = log.New(os.Stderr, "", log.LstdFlags)
	config := audit.Config{
		Client:         client,
		Author:         "Aaqibhafeezkhan",
		ReadmePath:     "README.md",
		LedgerPath:     "data/contributions.json",
		ExcludedOwners: audit.ParseExcludedOwners(os.Getenv("EXCLUDED_GITHUB_OWNERS")),
		Logger:         log.New(os.Stdout, "", 0),
	}
	if err := audit.Run(context.Background(), config); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
