package library_test

import (
	"encoding/json"
	"github.com/z354prance/overmynd/internal/library"
	"os"
	"testing"
)

func TestRoadmapReleaseCorpus(t *testing.T) {
	raw, err := os.ReadFile("testdata/roadmap_releases.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		ID, Source, Name, Title, Resolution, Folder, Limitation string
		Parse, Marked, Matches                                  bool
		Episode, Season                                         int
	}
	if err = json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	if len(fixtures) < 10 {
		t.Fatal("corpus was unexpectedly reduced")
	}
	ids := map[string]bool{}
	for _, f := range fixtures {
		if f.ID == "" || ids[f.ID] || f.Source == "" {
			t.Fatal("fixture needs unique ID and provenance", f.ID)
		}
		ids[f.ID] = true
		t.Run(f.ID, func(t *testing.T) {
			got, ok := library.Parse(f.Name)
			if ok != f.Parse {
				t.Fatalf("unexpected parse result: %+v, %v", got, ok)
			}
			if !ok {
				if f.Limitation == "" {
					t.Fatal("unhandled fixture has no reason")
				}
				return
			}
			if got.Title != f.Title || got.Episode != f.Episode || got.Marked != f.Marked || got.Res != f.Resolution {
				t.Fatalf("parser result changed: %+v", got)
			}
			if f.Marked && got.Season != f.Season {
				t.Fatalf("explicit season lost: %+v", got)
			}
			// Never bless the inherited season-1 default as an actual absolute mapping.
			if !f.Marked && (f.Season != 0 || f.Limitation == "") {
				t.Fatal("unmarked numbering requires an unresolved mapping note")
			}
			folder, _ := library.Match(got.Title, []string{f.Folder})
			if (folder != "") != f.Matches {
				t.Fatalf("unexpected folder match %q", folder)
			}
		})
	}
}
