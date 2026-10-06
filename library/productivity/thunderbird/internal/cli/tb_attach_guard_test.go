package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/server"
	"github.com/mvanhorn/printing-press-library/library/productivity/thunderbird/internal/mcp/cobratree"
)

type tbAttachFixture struct {
	home, docs, inside, outside string
}

func tbSetupAttach(t *testing.T, mcp bool) tbAttachFixture {
	t.Helper()
	b := tbSetupB(t, false, false)
	base := t.TempDir()
	f := tbAttachFixture{home: b.home, docs: filepath.Join(base, "Documents")}
	outDir := filepath.Join(base, "Outside")
	for _, d := range []string{f.docs, outDir, filepath.Join(base, "DocumentsX")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	f.inside = filepath.Join(f.docs, "report.txt")
	f.outside = filepath.Join(outDir, "secret.txt")
	for _, p := range []string{f.inside, f.outside, filepath.Join(base, "DocumentsX", "x.txt"), filepath.Join(f.docs, "second.txt")} {
		if err := os.WriteFile(p, []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	prev := tbDocumentsDir
	tbDocumentsDir = func() (string, error) { return f.docs, nil }
	t.Cleanup(func() { tbDocumentsDir = prev })
	surface := ""
	if mcp {
		surface = "mcp"
	}
	t.Setenv("THUNDERBIRD_LEARN_SURFACE", surface)
	t.Setenv(mcpBoundProfileEnv, "")
	return f
}

func tbDraftAttach(t *testing.T, f tbAttachFixture, attach string) (tbComposeSpec, string, error) {
	t.Helper()
	out, errOut, err := tbRun(t, f.home, "drafts", "new", "--to", "alice@example.com", "--attach="+attach, "--json")
	if err != nil {
		return tbComposeSpec{}, errOut + err.Error(), err
	}
	return tbDecode[tbComposeSpec](t, out), "", nil
}

func tbAssertAttachRejected(t *testing.T, f tbAttachFixture, attach string) {
	t.Helper()
	_, msg, err := tbDraftAttach(t, f, attach)
	if ExitCode(err) != 2 || !strings.Contains(msg, "Documents folder") {
		t.Fatalf("attach %q via MCP: exit %d, %s", attach, ExitCode(err), msg)
	}
}

func TestTBMCPAttachInsideDocumentsAccepted(t *testing.T) {
	f := tbSetupAttach(t, true)
	spec, msg, err := tbDraftAttach(t, f, f.inside+","+filepath.Join(f.docs, "sub", "..", "second.txt"))
	if err != nil {
		t.Fatal(msg)
	}
	if len(spec.Attachments) != 2 || !strings.EqualFold(spec.Attachments[0], f.inside) || !strings.EqualFold(spec.Attachments[1], filepath.Join(f.docs, "second.txt")) {
		t.Fatalf("attachments = %v", spec.Attachments)
	}
}

func TestTBMCPAttachOutsideDocumentsRejected(t *testing.T) {
	f := tbSetupAttach(t, true)
	tbAssertAttachRejected(t, f, f.outside)
	tbAssertAttachRejected(t, f, filepath.Join(filepath.Dir(f.docs), "DocumentsX", "x.txt"))
	tbAssertAttachRejected(t, f, f.inside+","+f.outside)
}

func TestTBMCPAttachBoundProfileSurfaceRestricted(t *testing.T) {
	f := tbSetupAttach(t, false)
	t.Setenv(mcpBoundProfileEnv, "default")
	if !tbMCPSurface() {
		t.Fatal("bound MCP profile not treated as MCP surface")
	}
	if _, err := tbResolveMCPAttachment(f.outside); err == nil {
		t.Fatal("outside file accepted")
	}
}

func TestTBMCPAttachTraversalRejected(t *testing.T) {
	f := tbSetupAttach(t, true)
	tbAssertAttachRejected(t, f, filepath.Join(f.docs, "..", "Outside", "secret.txt"))
}

func TestTBMCPAttachSymlinkOutsideRejected(t *testing.T) {
	f := tbSetupAttach(t, true)
	link := filepath.Join(f.docs, "link.txt")
	if err := os.Symlink(f.outside, link); err != nil {
		t.Skipf("symlink not permitted: %v", err)
	}
	tbAssertAttachRejected(t, f, link)
}

func TestTBMCPAttachJunctionOutsideRejected(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("junctions are Windows-only")
	}
	f := tbSetupAttach(t, true)
	junction := filepath.Join(f.docs, "junction")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", junction, filepath.Dir(f.outside)).CombinedOutput(); err != nil {
		t.Fatalf("mklink /J: %v %s", err, out)
	}
	tbAssertAttachRejected(t, f, filepath.Join(junction, "secret.txt"))
}

func TestTBTerminalAttachUnrestricted(t *testing.T) {
	f := tbSetupAttach(t, false)
	spec, msg, err := tbDraftAttach(t, f, f.outside)
	if err != nil {
		t.Fatal(msg)
	}
	if len(spec.Attachments) != 1 || spec.Attachments[0] != f.outside {
		t.Fatalf("attachments = %v", spec.Attachments)
	}
}

func TestTBDraftBodyFileHTMLExposedToMCP(t *testing.T) {
	root := RootCmd()
	s := server.NewMCPServer("test", "0.0.0")
	cobratree.RegisterAll(s, root, func() (string, error) { return "missing-binary", nil })
	tool, ok := s.ListTools()[cobratree.ToolNameForCommand(s, root, "drafts new")]
	if !ok {
		t.Fatal("no MCP tool for drafts new")
	}
	for _, name := range []string{"attach", "body-file", "html"} {
		if _, exposed := tool.Tool.InputSchema.Properties[name]; !exposed {
			t.Errorf("MCP schema does not expose %q", name)
		}
	}
}

func tbDraftBodyFile(t *testing.T, f tbAttachFixture, path string, extra ...string) (tbComposeSpec, string, error) {
	t.Helper()
	args := append([]string{"drafts", "new", "--to", "alice@example.com", "--body-file=" + path, "--json"}, extra...)
	out, errOut, err := tbRun(t, f.home, args...)
	if err != nil {
		return tbComposeSpec{}, errOut + err.Error(), err
	}
	return tbDecode[tbComposeSpec](t, out), "", nil
}

func TestTBMCPBodyFileInsideDocumentsAccepted(t *testing.T) {
	f := tbSetupAttach(t, true)
	if err := os.WriteFile(f.inside, []byte("<b>Hello</b>"), 0o600); err != nil {
		t.Fatal(err)
	}
	spec, msg, err := tbDraftBodyFile(t, f, f.inside, "--html")
	if err != nil {
		t.Fatal(msg)
	}
	if spec.Body != "<b>Hello</b>" || !spec.HTML {
		t.Fatalf("draft = %+v", spec)
	}
}

func TestTBMCPBodyFileOutsideDocumentsRejected(t *testing.T) {
	f := tbSetupAttach(t, true)
	for _, p := range []string{f.outside, filepath.Join(f.docs, "..", "Outside", "secret.txt"), filepath.Join(filepath.Dir(f.docs), "DocumentsX", "x.txt")} {
		_, msg, err := tbDraftBodyFile(t, f, p)
		if ExitCode(err) != 2 || !strings.Contains(msg, "Documents folder") {
			t.Fatalf("body-file %q via MCP: exit %d, %s", p, ExitCode(err), msg)
		}
	}
}

func TestTBTerminalBodyFileUnrestricted(t *testing.T) {
	f := tbSetupAttach(t, false)
	spec, msg, err := tbDraftBodyFile(t, f, f.outside)
	if err != nil {
		t.Fatal(msg)
	}
	if spec.Body != "data" || spec.HTML {
		t.Fatalf("draft = %+v", spec)
	}
}

func TestTBSplitMCPAttachmentsKeepsCommaInFilename(t *testing.T) {
	dir := t.TempDir()
	comma := filepath.Join(dir, "report,2024.pdf")
	other := filepath.Join(dir, "b.pdf")
	for _, p := range []string{comma, other} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if got := tbSplitMCPAttachments([]string{comma}); len(got) != 1 || got[0] != comma {
		t.Fatalf("comma filename split: %v", got)
	}
	if got := tbSplitMCPAttachments([]string{other + "," + other}); len(got) != 2 {
		t.Fatalf("list of two files not split: %v", got)
	}
}

func TestTBBuildDraftEMLHTMLContentType(t *testing.T) {
	att := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(att, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		html   bool
		attach []string
		want   string
	}{
		{true, nil, "text/html"},
		{false, nil, "text/plain"},
		{true, []string{att}, "text/html"},
		{false, []string{att}, "text/plain"},
	} {
		eml, err := tbBuildDraftEML(&tbComposeSpec{To: []string{"a@b.it"}, Body: "<b>x</b>", HTML: tc.html, Attachments: tc.attach}, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(eml), "Content-Type: "+tc.want+";") {
			t.Fatalf("html=%v attach=%v: want %s in\n%s", tc.html, tc.attach, tc.want, eml)
		}
	}
}
