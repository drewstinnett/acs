package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/drewstinnett/acs/internal/web"
	"github.com/spf13/cobra"
)

var (
	servePort int
	serveHost string
	serveOpen bool
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the web browser interface",
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := openDB()
		if err != nil {
			return err
		}
		defer db.Close()

		addr := fmt.Sprintf("%s:%d", serveHost, servePort)
		url := fmt.Sprintf("http://%s:%d", serveHost, servePort)
		if serveHost == "" || serveHost == "0.0.0.0" {
			url = fmt.Sprintf("http://localhost:%d", servePort)
		}

		fmt.Fprintf(os.Stdout, "ACS Archive Browser running at %s\n", url)
		fmt.Fprintln(os.Stdout, "Press Ctrl+C to stop.")

		if serveOpen {
			openBrowser(url)
		}

		return web.ListenAndServe(addr, db, dataDir, log)
	},
}

func init() {
	serveCmd.Flags().IntVar(&servePort, "port", 8080, "port to listen on")
	serveCmd.Flags().StringVar(&serveHost, "host", "127.0.0.1", "host address to bind")
	serveCmd.Flags().BoolVar(&serveOpen, "open", false, "open browser automatically")
}

func openBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "linux":
		cmd = "xdg-open"
		args = []string{url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	default:
		return
	}
	go exec.Command(cmd, args...).Start() //nolint:errcheck
}
