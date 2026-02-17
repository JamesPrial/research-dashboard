package pathutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jamesprial/research-dashboard/internal/model"
	"github.com/jamesprial/research-dashboard/internal/pathutil"
)

// ===========================================================================
// Test: ValidateDirName
// ===========================================================================

func Test_ValidateDirName_Cases(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid name with date suffix",
			input:   "research-ai-safety-20240101",
			wantErr: false,
		},
		{
			name:    "valid simple name",
			input:   "research-test",
			wantErr: false,
		},
		{
			name:    "contains forward slash",
			input:   "research-test/foo",
			wantErr: true,
		},
		{
			name:    "contains backslash",
			input:   "research-test\\foo",
			wantErr: true,
		},
		{
			name:    "contains dot-dot traversal",
			input:   "research-test/../etc",
			wantErr: true,
		},
		{
			name:    "just dot-dot",
			input:   "..",
			wantErr: true,
		},
		{
			name:    "no research prefix",
			input:   "output-data",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "just research- prefix",
			input:   "research-",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := pathutil.ValidateDirName(tt.input)
			if tt.wantErr && err == nil {
				t.Errorf("ValidateDirName(%q) = nil, want error", tt.input)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ValidateDirName(%q) = %v, want nil", tt.input, err)
			}
		})
	}
}

// Additional edge cases for ValidateDirName.
func Test_ValidateDirName_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "dot-dot at end",
			input:   "research-test/..",
			wantErr: true,
		},
		{
			name:    "single dot",
			input:   ".",
			wantErr: true, // no research- prefix
		},
		{
			name:    "research prefix exact",
			input:   "research-",
			wantErr: false,
		},
		{
			name:    "research without hyphen",
			input:   "researchdata",
			wantErr: true, // must start with "research-"
		},
		{
			name:    "mixed case Research-test",
			input:   "Research-test",
			wantErr: true, // prefix is case-sensitive
		},
		{
			name:    "spaces in name",
			input:   "research-my project",
			wantErr: false, // no spec says spaces are invalid; only slash, backslash, dot-dot, and prefix are checked
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := pathutil.ValidateDirName(tt.input)
			if tt.wantErr && err == nil {
				t.Errorf("ValidateDirName(%q) = nil, want error", tt.input)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ValidateDirName(%q) = %v, want nil", tt.input, err)
			}
		})
	}
}

// Verify the error returned by ValidateDirName is descriptive (non-empty).
func Test_ValidateDirName_ErrorMessages(t *testing.T) {
	invalidInputs := []string{
		"",
		"..",
		"output-data",
		"research-test/foo",
		"research-test\\foo",
	}

	for _, input := range invalidInputs {
		t.Run(input, func(t *testing.T) {
			err := pathutil.ValidateDirName(input)
			if err == nil {
				t.Fatalf("ValidateDirName(%q) = nil, want error", input)
			}
			if err.Error() == "" {
				t.Errorf("ValidateDirName(%q) returned error with empty message", input)
			}
		})
	}
}

// ===========================================================================
// Test: ResolveSafeFile
// ===========================================================================

func Test_ResolveSafeFile_Cases(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		wantErr  bool
		wantBase string // expected filename relative to baseDir (if no error)
	}{
		{
			name:     "simple file",
			filePath: "report.md",
			wantErr:  false,
			wantBase: "report.md",
		},
		{
			name:     "nested file",
			filePath: "sources/001-example.md",
			wantErr:  false,
			wantBase: filepath.Join("sources", "001-example.md"),
		},
		{
			name:     "dot-dot traversal",
			filePath: "../etc/passwd",
			wantErr:  true,
		},
		{
			name:     "dot-dot in middle",
			filePath: "sources/../../etc/passwd",
			wantErr:  true,
		},
		{
			name:     "absolute path attempt",
			filePath: "/etc/passwd",
			wantErr:  true,
		},
		{
			name:     "clean dot-slash path",
			filePath: "./report.md",
			wantErr:  false,
			wantBase: "report.md",
		},
		{
			name:     "empty file path",
			filePath: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir := t.TempDir()

			got, err := pathutil.ResolveSafeFile(baseDir, tt.filePath)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ResolveSafeFile(%q, %q) = (%q, nil), want error",
						baseDir, tt.filePath, got)
				}
				return
			}

			if err != nil {
				t.Fatalf("ResolveSafeFile(%q, %q) unexpected error: %v",
					baseDir, tt.filePath, err)
			}

			// The resolved path must be within baseDir.
			wantPath := filepath.Join(baseDir, tt.wantBase)
			if got != wantPath {
				t.Errorf("ResolveSafeFile(%q, %q) = %q, want %q",
					baseDir, tt.filePath, got, wantPath)
			}
		})
	}
}

// Verify the resolved path is always under baseDir (containment check).
func Test_ResolveSafeFile_Containment(t *testing.T) {
	baseDir := t.TempDir()

	safeInputs := []string{
		"report.md",
		"./report.md",
		"sources/001-example.md",
		"deep/nested/path/file.html",
	}

	for _, input := range safeInputs {
		t.Run(input, func(t *testing.T) {
			got, err := pathutil.ResolveSafeFile(baseDir, input)
			if err != nil {
				t.Fatalf("ResolveSafeFile(%q, %q) unexpected error: %v",
					baseDir, input, err)
			}

			// The resolved path must have baseDir as a prefix.
			rel, err := filepath.Rel(baseDir, got)
			if err != nil {
				t.Fatalf("filepath.Rel(%q, %q) error: %v", baseDir, got, err)
			}
			// rel must not start with ".." which would escape baseDir.
			if len(rel) >= 2 && rel[:2] == ".." {
				t.Errorf("resolved path %q escapes base dir %q (rel=%q)",
					got, baseDir, rel)
			}
		})
	}
}

// Verify traversal attacks are blocked even with complex paths.
func Test_ResolveSafeFile_TraversalVariants(t *testing.T) {
	baseDir := t.TempDir()

	attacks := []string{
		"../etc/passwd",
		"../../etc/shadow",
		"sources/../../etc/passwd",
		"a/b/c/../../../../etc/passwd",
		"../../../../../../../etc/passwd",
	}

	for _, attack := range attacks {
		t.Run(attack, func(t *testing.T) {
			_, err := pathutil.ResolveSafeFile(baseDir, attack)
			if err == nil {
				t.Errorf("ResolveSafeFile(%q, %q) = nil error, want error for traversal attempt",
					baseDir, attack)
			}
		})
	}
}

// Verify that ResolveSafeFile returns an absolute path on success.
func Test_ResolveSafeFile_ReturnsAbsolutePath(t *testing.T) {
	baseDir := t.TempDir()

	got, err := pathutil.ResolveSafeFile(baseDir, "report.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !filepath.IsAbs(got) {
		t.Errorf("ResolveSafeFile returned non-absolute path: %q", got)
	}
}

// Verify that ResolveSafeFile works when baseDir has a trailing slash.
func Test_ResolveSafeFile_BaseDirTrailingSlash(t *testing.T) {
	baseDir := t.TempDir()
	baseDirSlash := baseDir + string(os.PathSeparator)

	got, err := pathutil.ResolveSafeFile(baseDirSlash, "report.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := filepath.Join(baseDir, "report.md")
	if got != want {
		t.Errorf("ResolveSafeFile with trailing slash = %q, want %q", got, want)
	}
}

// ===========================================================================
// Test: ClassifyFileType
// ===========================================================================

func Test_ClassifyFileType_Cases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  model.FileType
	}{
		{
			name:  "markdown",
			input: "report.md",
			want:  model.FileTypeMD,
		},
		{
			name:  "html",
			input: "page.html",
			want:  model.FileTypeHTML,
		},
		{
			name:  "htm",
			input: "page.htm",
			want:  model.FileTypeHTML,
		},
		{
			name:  "uppercase MD",
			input: "FILE.MD",
			want:  model.FileTypeMD,
		},
		{
			name:  "uppercase HTML",
			input: "PAGE.HTML",
			want:  model.FileTypeHTML,
		},
		{
			name:  "other extension json",
			input: "data.json",
			want:  model.FileTypeOther,
		},
		{
			name:  "no extension",
			input: "README",
			want:  model.FileTypeOther,
		},
		{
			name:  "multiple dots - last extension wins",
			input: "report.backup.md",
			want:  model.FileTypeMD,
		},
		{
			name:  "empty string",
			input: "",
			want:  model.FileTypeOther,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pathutil.ClassifyFileType(tt.input)
			if got != tt.want {
				t.Errorf("ClassifyFileType(%q) = %q, want %q",
					tt.input, got, tt.want)
			}
		})
	}
}

// Additional edge cases for ClassifyFileType.
func Test_ClassifyFileType_EdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  model.FileType
	}{
		{
			name:  "mixed case Md",
			input: "report.Md",
			want:  model.FileTypeMD,
		},
		{
			name:  "mixed case HtMl",
			input: "page.HtMl",
			want:  model.FileTypeHTML,
		},
		{
			name:  "mixed case Htm",
			input: "page.Htm",
			want:  model.FileTypeHTML,
		},
		{
			name:  "dot only",
			input: ".",
			want:  model.FileTypeOther,
		},
		{
			name:  "hidden file .md extension",
			input: ".hidden.md",
			want:  model.FileTypeMD,
		},
		{
			name:  "hidden file no extension",
			input: ".gitignore",
			want:  model.FileTypeOther,
		},
		{
			name:  "path with directories",
			input: "sources/report.md",
			want:  model.FileTypeMD,
		},
		{
			name:  "txt extension",
			input: "notes.txt",
			want:  model.FileTypeOther,
		},
		{
			name:  "pdf extension",
			input: "paper.pdf",
			want:  model.FileTypeOther,
		},
		{
			name:  "trailing dot",
			input: "report.",
			want:  model.FileTypeOther,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pathutil.ClassifyFileType(tt.input)
			if got != tt.want {
				t.Errorf("ClassifyFileType(%q) = %q, want %q",
					tt.input, got, tt.want)
			}
		})
	}
}

// Verify ClassifyFileType always returns a valid FileType constant.
func Test_ClassifyFileType_AlwaysReturnsValidType(t *testing.T) {
	validTypes := map[model.FileType]bool{
		model.FileTypeMD:    true,
		model.FileTypeHTML:  true,
		model.FileTypeOther: true,
	}

	inputs := []string{
		"report.md", "page.html", "page.htm", "FILE.MD", "PAGE.HTML",
		"data.json", "README", "report.backup.md", "", ".",
		".hidden", "notes.txt", "archive.tar.gz",
	}

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			got := pathutil.ClassifyFileType(input)
			if !validTypes[got] {
				t.Errorf("ClassifyFileType(%q) = %q, which is not a valid FileType constant",
					input, got)
			}
		})
	}
}
