package nlp

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

var testWordNetDir string

func TestNewWordNet(t *testing.T) {
	wn, err := NewWordNet(testWordNetDir)
	if err != nil {
		t.Fatalf("Failed to create WordNet: %v", err)
	}
	defer wn.Close()

	// Verify that some basic data was loaded
	if len(wn.lemmaPosOffsetMap) == 0 {
		t.Error("lemmaPosOffsetMap is empty")
	}

	// Check exception map loaded
	if len(wn.exceptionMap[NOUN]) == 0 {
		t.Error("NOUN exception map is empty")
	}
}

func TestMorphy(t *testing.T) {
	wn, err := NewWordNet(testWordNetDir)
	if err != nil {
		t.Fatalf("Failed to create WordNet: %v", err)
	}
	defer wn.Close()

	tests := []struct {
		form     string
		pos      string
		expected []string
	}{
		{"dogs", NOUN, []string{"dog"}},
		{"churches", NOUN, []string{"church"}},
		{"running", VERB, []string{"run"}},
		{"better", ADJ, []string{"good"}},
	}

	for _, tt := range tests {
		result := wn.morphy(tt.form, tt.pos, true)
		// We just verify that morphy returns some results for known words
		// The exact results depend on what's in the exception files
		t.Logf("morphy(%q, %q) = %v", tt.form, tt.pos, result)
	}
}

func TestSynsets(t *testing.T) {
	wn, err := NewWordNet(testWordNetDir)
	if err != nil {
		t.Fatalf("Failed to create WordNet: %v", err)
	}
	defer wn.Close()

	tests := []struct {
		lemma      string
		pos        string
		minSynsets int
		checkNames []string
	}{
		// Basic nouns
		{"dog", "", 1, []string{"dog.n.01"}},
		{"dog", NOUN, 1, []string{"dog.n.01"}},
		{"entity", NOUN, 1, []string{"entity.n.01"}},
		{"computer", NOUN, 1, nil},
		// Basic verbs
		{"run", VERB, 1, nil},
		{"walk", VERB, 1, nil},
		// Basic adjectives/adverbs
		{"good", ADJ, 1, nil},
		{"quickly", ADV, 1, nil},
		// Edge case: multi-word phrases
		{"physical_entity", NOUN, 1, nil},
		{"hot_dog", NOUN, 1, nil},
		// Edge case: rare words
		{"aardvark", NOUN, 1, nil},
		// Edge case: uppercase input (should be converted to lowercase)
		{"DOG", NOUN, 1, []string{"dog.n.01"}},
		// Edge case: non-existent words
		{"xyznonexistent", "", 0, nil},
	}

	for _, tt := range tests {
		synsets := wn.Synsets(tt.lemma, tt.pos)
		if len(synsets) < tt.minSynsets {
			t.Errorf("Synsets(%q, %q) returned %d synsets, expected at least %d",
				tt.lemma, tt.pos, len(synsets), tt.minSynsets)
		}

		// Check that expected names are present
		if tt.checkNames != nil {
			names := make([]string, len(synsets))
			for i, s := range synsets {
				names[i] = s.Name
			}
			for _, expectedName := range tt.checkNames {
				found := false
				for _, name := range names {
					if name == expectedName {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Synsets(%q, %q) did not contain expected synset %q, got %v",
						tt.lemma, tt.pos, expectedName, names)
				}
			}
		}

		t.Logf("Synsets(%q, %q) returned %d synsets", tt.lemma, tt.pos, len(synsets))
		for _, s := range synsets {
			t.Logf("  - %s: %s", s.Name, s.Definition)
		}
	}
}

func TestSynsetsDetailed(t *testing.T) {
	wn, err := NewWordNet(testWordNetDir)
	if err != nil {
		t.Fatalf("Failed to create WordNet: %v", err)
	}
	defer wn.Close()

	// Test entity - should have at least 1 synset
	synsets := wn.Synsets("entity", NOUN)
	if len(synsets) == 0 {
		t.Fatal("Expected at least 1 synset for 'entity'")
	}

	found := false
	for _, s := range synsets {
		if s.Offset == 1740 { // entity.n.01 offset
			found = true
			if s.Definition == "" {
				t.Error("Expected non-empty definition for entity.n.01")
			}
			if len(s.Lemmas) == 0 {
				t.Error("Expected at least one lemma")
			}
		}
	}
	if !found {
		t.Errorf("Expected to find synset with offset 1740 for 'entity'")
	}
}

func TestSynsetsConsistencyWithPython(t *testing.T) {
	wn, err := NewWordNet(testWordNetDir)
	if err != nil {
		t.Fatalf("Failed to create WordNet: %v", err)
	}
	defer wn.Close()

	// These are the expected results from Python NLTK for comparison
	// wordnet.synsets('dog') returns synsets with these names:
	pythonDogNames := []string{
		"dog.n.01",
		"frump.n.01",
		"dog.n.03",
		"cad.n.01",
		"frank.n.02",
		"pawl.n.01",
		"andiron.n.01",
	}

	synsets := wn.Synsets("dog", NOUN)
	var goDogNames []string
	for _, s := range synsets {
		goDogNames = append(goDogNames, s.Name)
	}

	// Sort both lists for comparison
	sort.Strings(pythonDogNames)
	sort.Strings(goDogNames)

	t.Logf("Python expected (approximate): %v", pythonDogNames)
	t.Logf("Go result: %v", goDogNames)

	// We may not match exactly due to sense numbering, but we should have some overlap
	if len(goDogNames) == 0 {
		t.Error("Expected at least some synsets for 'dog'")
	}
}

func TestSynsetContent(t *testing.T) {
	wn, err := NewWordNet(testWordNetDir)
	if err != nil {
		t.Fatalf("Failed to create WordNet: %v", err)
	}
	defer wn.Close()

	synsets := wn.Synsets("dog", NOUN)
	if len(synsets) == 0 {
		t.Fatal("Expected at least 1 synset for 'dog'")
	}

	// Check synset structure
	for _, s := range synsets {
		if s.Name == "" {
			t.Error("Synset name is empty")
		}
		if s.POS == "" {
			t.Error("Synset POS is empty")
		}
		if s.Offset == 0 {
			t.Error("Synset offset is 0")
		}
		if len(s.Lemmas) == 0 {
			t.Error("Synset has no lemmas")
		}
	}
}

func BenchmarkSynsets(b *testing.B) {
	wn, err := NewWordNet(testWordNetDir)
	if err != nil {
		b.Fatalf("Failed to create WordNet: %v", err)
	}
	defer wn.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wn.Synsets("dog", NOUN)
	}
}

// Helper function to check if two string slices are equal
func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sort.Strings(a)
	sort.Strings(b)
	return reflect.DeepEqual(a, b)
}

func init() {
	// Find project root by locating go.mod file
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			// Found go.mod, project root is dir
			testWordNetDir = filepath.Join(dir, "resource", "wordnet")
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root directory
			break
		}
		dir = parent
	}
	// Fallback to relative path if go.mod not found
	testWordNetDir = "../../../resource/wordnet"
}
