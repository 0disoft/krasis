package peerupdate

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// Preserve the previous linear implementation as an independent oracle and
// benchmark baseline for the indexed plan. It never removes any files.
func linearPlanPruneFolders(folders []pruneFolder, keep []string, active, base string, now time.Time) []string {
	protected := make([]string, 0, len(keep)+1)
	for _, path := range keep {
		protected = append(protected, filepath.Clean(path))
	}
	if active != "" {
		protected = append(protected, active)
	}
	var names []string
	for i, folder := range folders {
		if i < pruneNewestToKeep || now.Sub(folder.at) < pruneRetentionAge {
			continue
		}
		path := filepath.Join(base, folder.name)
		if slices.ContainsFunc(protected, func(p string) bool { return strings.EqualFold(p, path) }) {
			continue
		}
		names = append(names, folder.name)
	}
	return names
}

func TestPlanPruneFoldersIndexMatchesLinear(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	for _, root := range []string{"stages", "stages-KΣS", "stages-İı"} {
		base := filepath.Join(root, "updates")
		// Unicode simple folding includes Kelvin sign, final sigma and long s.
		foldedBase := strings.NewReplacer("K", "K", "Σ", "ς", "S", "ſ").Replace(base)
		var folders []pruneFolder
		for i, age := range []time.Duration{
			48 * time.Hour, 48 * time.Hour, 24*time.Hour - 1,
			24 * time.Hour, 24*time.Hour + 1, 48 * time.Hour,
			48 * time.Hour, 48 * time.Hour, 48 * time.Hour,
		} {
			folders = append(folders, pruneFolder{fmt.Sprintf("%032x", i+10), now.Add(-age)})
		}
		// A plan must keep input order and repeated removal candidates intact.
		folders = append(folders, folders[8])
		keep := []string{
			filepath.Join(foldedBase, strings.ToUpper(folders[5].name)),
			filepath.Join(base, folders[5].name), // Duplicate protection hint.
			base + string(filepath.Separator) + "unused" + string(filepath.Separator) + ".." + string(filepath.Separator) + folders[3].name,
			filepath.Join(base+"-other", folders[7].name), // Same ID, other root.
			filepath.Join(root, "other", folders[8].name),
		}
		active := filepath.Join(foldedBase, strings.ToUpper(folders[6].name))
		for i, value := range []string{"", active, active + string(filepath.Separator), active + string(filepath.Separator) + "."} {
			t.Run(fmt.Sprintf("%s/active=%d", root, i), func(t *testing.T) {
				before, keepBefore := slices.Clone(folders), slices.Clone(keep)
				want := linearPlanPruneFolders(folders, keep, value, base, now)
				got := planPruneFolders(folders, keep, value, base, now)
				if !slices.Equal(got, want) {
					t.Fatalf("plan=%v want=%v", got, want)
				}
				if !slices.Equal(folders, before) || !slices.Equal(keep, keepBefore) {
					t.Fatal("plan mutated caller input")
				}
			})
		}
	}
}
