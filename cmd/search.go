package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/drewstinnett/acs/internal/archive"
	"github.com/drewstinnett/acs/internal/store"
	"github.com/spf13/cobra"
)

var (
	searchSave    bool
	searchLimit   int
	searchSubject string
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search Internet Archive for ACS items",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.Join(args, " ")

		client := archive.NewClient(log)
		results, err := client.Search(cmd.Context(), query, searchLimit)
		if err != nil {
			return fmt.Errorf("search: %w", err)
		}

		if len(results) == 0 {
			fmt.Println("No results found.")
			return nil
		}

		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "IDENTIFIER\tTITLE\tDATE")
		for _, r := range results {
			title := r.Title.First()
			if len(title) > 60 {
				title = title[:57] + "..."
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\n", r.Identifier, title, r.Date.First())
		}
		tw.Flush()
		fmt.Printf("\n%d result(s)\n", len(results))

		if searchSave {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()

			saved := 0
			for _, r := range results {
				date := r.Date.First()
				it := store.Item{
					ID:          r.Identifier,
					Title:       r.Title.First(),
					Description: r.Description.First(),
					Date:        date,
					Year:        extractYear(date),
					Subject:     []string(r.Subject),
				}
				if err := db.UpsertItem(it); err != nil {
					log.Warn("failed to save item", "id", r.Identifier, "err", err)
					continue
				}
				saved++
			}
			fmt.Printf("Saved %d item(s) to database.\n", saved)
		}

		return nil
	},
}

func init() {
	searchCmd.Flags().BoolVar(&searchSave, "save", false, "save results to the local database")
	searchCmd.Flags().IntVar(&searchLimit, "limit", 50, "maximum number of results")
	searchCmd.Flags().StringVar(&searchSubject, "subject", "", "filter by subject")
}

// openDB opens the SQLite database, creating parent dirs as needed.
func openDB() (*store.DB, error) {
	if err := mkdirFor(dbPath); err != nil {
		return nil, err
	}
	db, err := store.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database %q: %w", dbPath, err)
	}
	return db, nil
}

func mkdirFor(path string) error {
	dir := path[:max(strings.LastIndex(path, "/"), strings.LastIndex(path, "\\"))]
	if dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

func extractYear(date string) int {
	if len(date) >= 4 {
		y, err := strconv.Atoi(date[:4])
		if err == nil {
			return y
		}
	}
	return 0
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
