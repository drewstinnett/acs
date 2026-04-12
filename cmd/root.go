package cmd

import (
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	dbPath  string
	dataDir string
	verbose bool
	log     *slog.Logger
)

var rootCmd = &cobra.Command{
	Use:   "acs",
	Short: "ACS Archive: download, OCR, and browse ACS microfilm from Internet Archive",
	Long: `acs downloads microfilm images from the Internet Archive,
runs OCR with Tesseract, stores results in a local SQLite database,
and serves a web interface for browsing and searching the content.

External dependencies required: tesseract-ocr, imagemagick`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		level := slog.LevelInfo
		if verbose {
			level = slog.LevelDebug
		}
		log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	},
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	home, _ := os.UserHomeDir()
	defaultData := filepath.Join(home, ".acs")

	rootCmd.PersistentFlags().StringVar(&dbPath, "db", filepath.Join(defaultData, "acs.db"),
		"path to SQLite database")
	rootCmd.PersistentFlags().StringVar(&dataDir, "data-dir", filepath.Join(defaultData, "images"),
		"directory for downloaded image files")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "enable debug logging")

	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(downloadCmd)
	rootCmd.AddCommand(ocrCmd)
	rootCmd.AddCommand(serveCmd)
}
