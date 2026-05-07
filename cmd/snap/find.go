// `snap find <query>` -- search the store. mirrors a stripped-down
// version of `snap find` from upstream: just the matched name,
// version, publisher, and summary.

package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/snapcore/snapd/store"
)

func cmdFind(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("find needs a query")
	}
	st := store.New(nil, nil)
	ctx := context.Background()
	results, err := st.Find(ctx, &store.Search{Query: strings.Join(args, " ")}, nil)
	if err != nil {
		return err
	}
	if len(results) == 0 {
		fmt.Println("no matches")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Name\tVersion\tPublisher\tSummary")
	for _, r := range results {
		pub := "(unknown)"
		if r.Publisher.Username != "" {
			pub = r.Publisher.Username
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.SnapName(), r.Version, pub, r.Summary())
	}
	return w.Flush()
}
