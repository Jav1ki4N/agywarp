package components

import (
	"testing"

	"agywarp/internal/process"

	tea "charm.land/bubbletea/v2"
)

func TestGroupEditorCreateAndEdit(t *testing.T) {
	editor := NewGroupEditor()
	editor.Height = 10
	editor.Width = 60

	// 1. Test OpenCreate
	editor.OpenCreate()
	if !editor.Open || editor.IsEditing {
		t.Fatal("expected editor to be open in create mode")
	}

	// Supply mock running processes so /bin/foo and /bin/bar pass system validation
	editor.SetRunningProcesses([]process.RunningProcess{
		{PID: 100, Name: "foo", Executable: "/bin/foo"},
		{PID: 101, Name: "bar", Executable: "/bin/bar"},
	})

	// Type Group Name: "MyGroup"
	for _, ch := range "MyGroup" {
		editor.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}
	if editor.NameInput != "MyGroup" {
		t.Fatalf("expected NameInput to be 'MyGroup', got '%s'", editor.NameInput)
	}

	// Press Tab or Enter to jump to Add Path input
	editor.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if editor.ActiveField != 2 {
		t.Fatalf("expected ActiveField to be 2 (Add Path), got %d", editor.ActiveField)
	}

	// Type Path 1: "/bin/foo"
	for _, ch := range "/bin/foo" {
		editor.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}
	// Press Enter to append path
	editor.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(editor.Paths) != 1 || editor.Paths[0] != "/bin/foo" {
		t.Fatalf("expected 1 path '/bin/foo', got %v", editor.Paths)
	}

	// Type Path 2: "/bin/bar"
	for _, ch := range "/bin/bar" {
		editor.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}
	// Press Enter to append path
	editor.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(editor.Paths) != 2 || editor.Paths[1] != "/bin/bar" {
		t.Fatalf("expected 2 paths, got %v", editor.Paths)
	}

	// Save with Ctrl+S
	cmd := editor.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("expected save cmd to be returned")
	}
	msg := cmd()
	savedMsg, ok := msg.(GroupSavedMsg)
	if !ok {
		t.Fatalf("expected GroupSavedMsg, got %T", msg)
	}
	if savedMsg.Profile.Label != "MyGroup" {
		t.Errorf("expected label 'MyGroup', got '%s'", savedMsg.Profile.Label)
	}
	if len(savedMsg.Profile.Matchers) != 2 {
		t.Fatalf("expected 2 matchers, got %d", len(savedMsg.Profile.Matchers))
	}
	if savedMsg.Profile.Matchers[0].Pattern != "/bin/foo" || savedMsg.Profile.Matchers[1].Pattern != "/bin/bar" {
		t.Errorf("matchers mismatch: %v", savedMsg.Profile.Matchers)
	}

	// 2. Test OpenEdit
	existingProf := savedMsg.Profile
	editor.OpenEdit(existingProf)
	if !editor.Open || !editor.IsEditing || editor.EditingID != existingProf.ID {
		t.Fatal("expected editor to be open in edit mode")
	}
	if len(editor.Paths) != 2 {
		t.Fatalf("expected 2 paths in edit mode, got %d", len(editor.Paths))
	}

	// Switch to Paths list (Field 1) and remove first path with 'd'
	editor.ActiveField = 1
	editor.PathCursor = 0
	editor.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if len(editor.Paths) != 1 || editor.Paths[0] != "/bin/bar" {
		t.Fatalf("expected 1 remaining path '/bin/bar', got %v", editor.Paths)
	}
}

func TestGroupEditorPaste(t *testing.T) {
	editor := NewGroupEditor()
	editor.OpenCreate()

	// 1. Paste into Group Name (ActiveField == 0)
	editor.Update(tea.PasteMsg{Content: "Antigravity IDE\n"})
	if editor.NameInput != "Antigravity IDE" {
		t.Fatalf("expected NameInput to be 'Antigravity IDE', got %q", editor.NameInput)
	}

	// 2. Switch to Add Path (ActiveField == 2)
	editor.ActiveField = 2

	// Paste a single path with trailing newline (common when copying from terminal)
	editor.Update(tea.PasteMsg{Content: "/home/i4N/.gemini/bin/agy\n"})
	if editor.PathInput != "/home/i4N/.gemini/bin/agy" {
		t.Fatalf("expected PathInput to be '/home/i4N/.gemini/bin/agy', got %q", editor.PathInput)
	}

	// Commit with Enter
	editor.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(editor.Paths) != 1 || editor.Paths[0] != "/home/i4N/.gemini/bin/agy" {
		t.Fatalf("expected 1 path committed, got %v", editor.Paths)
	}

	// 3. Multi-line batch paste directly into paths list
	multiLine := `/home/i4N/.local/bin/agy
/usr/bin/curl
/usr/bin/git`
	editor.Update(tea.PasteMsg{Content: multiLine})
	if len(editor.Paths) != 4 {
		t.Fatalf("expected 4 paths after multi-line paste, got %d (%v)", len(editor.Paths), editor.Paths)
	}
	if editor.Paths[1] != "/home/i4N/.local/bin/agy" || editor.Paths[2] != "/usr/bin/curl" || editor.Paths[3] != "/usr/bin/git" {
		t.Fatalf("unexpected paths content: %v", editor.Paths)
	}

	// 4. Unicode typing & rune backspace
	editor.ActiveField = 0
	editor.NameInput = ""
	// Append Chinese text
	editor.Update(tea.PasteMsg{Content: "网关测试"})
	if editor.NameInput != "网关测试" {
		t.Fatalf("expected NameInput '网关测试', got %q", editor.NameInput)
	}
	// Backspace once -> should delete last rune '试', leaving '网关测'
	editor.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if editor.NameInput != "网关测" {
		t.Fatalf("expected '网关测' after backspace, got %q", editor.NameInput)
	}
}

func TestGroupEditorPathValidation(t *testing.T) {
	editor := NewGroupEditor()
	editor.OpenCreate()
	editor.ActiveField = 2

	// 1. Enter non-existent path -> should fail validation and log ERR
	editor.PathInput = "/non/existent/path/for/testing/12345"
	cmd := editor.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected error log cmd to be returned")
	}
	msg := cmd()
	logMsg, ok := msg.(LogMsg)
	if !ok || logMsg.Level != "ERR" {
		t.Fatalf("expected LogMsg with level 'ERR', got %T (%v)", msg, msg)
	}
	if len(editor.Paths) != 0 {
		t.Fatalf("invalid path must NOT be added to Paths list on single Enter, got %v", editor.Paths)
	}
	if editor.ErrorMsg == "" {
		t.Fatal("expected ErrorMsg to be set on validation failure")
	}

	// 2. Press Enter again on the same path -> force add with WARN
	cmd = editor.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected force add warning log cmd")
	}
	msg = cmd()
	logMsg, ok = msg.(LogMsg)
	if !ok || logMsg.Level != "WARN" {
		t.Fatalf("expected LogMsg with level 'WARN', got %T (%v)", msg, msg)
	}
	if len(editor.Paths) != 1 || editor.Paths[0] != "/non/existent/path/for/testing/12345" {
		t.Fatalf("expected force added path in list, got %v", editor.Paths)
	}
	if editor.ErrorMsg != "" {
		t.Fatalf("expected ErrorMsg to be cleared after force add, got %q", editor.ErrorMsg)
	}

	// 3. Enter valid path (e.g. "sh") -> should succeed and log OK
	editor.PathInput = "sh"
	cmd = editor.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected OK log cmd")
	}
	msg = cmd()
	logMsg, ok = msg.(LogMsg)
	if !ok || logMsg.Level != "OK" {
		t.Fatalf("expected LogMsg with level 'OK', got %T (%v)", msg, msg)
	}
	if len(editor.Paths) != 2 || editor.Paths[1] != "sh" {
		t.Fatalf("expected 'sh' to be added to list, got %v", editor.Paths)
	}
}

func TestGroupEditorFunctionalKeyInput(t *testing.T) {
	editor := NewGroupEditor()
	editor.OpenCreate()

	// 1. Test typing words containing 'k', 'j', 'd' into NameInput (ActiveField == 0)
	editor.ActiveField = 0
	for _, ch := range "docker-java-kind" {
		editor.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}
	if editor.NameInput != "docker-java-kind" {
		t.Fatalf("expected 'docker-java-kind', got %q (some keys were swallowed!)", editor.NameInput)
	}

	// 2. Test typing words into PathInput (ActiveField == 2)
	editor.ActiveField = 2
	for _, ch := range "fuck" {
		editor.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}
	if editor.PathInput != "fuck" {
		t.Fatalf("expected PathInput to be 'fuck', got %q ('k' was swallowed!)", editor.PathInput)
	}

	// Clear and test another word with 'd' and 'j': "dj-daemon"
	editor.PathInput = ""
	for _, ch := range "dj-daemon" {
		editor.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}
	if editor.PathInput != "dj-daemon" {
		t.Fatalf("expected PathInput to be 'dj-daemon', got %q ('d' or 'j' was swallowed!)", editor.PathInput)
	}

	// 3. Test that 'k', 'j', 'd' DO work as navigation/delete when ActiveField == 1 (Paths list)
	editor.Paths = []string{"/bin/p1", "/bin/p2", "/bin/p3"}
	editor.ActiveField = 1
	editor.PathCursor = 0

	// 'j' moves cursor down
	editor.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if editor.PathCursor != 1 {
		t.Fatalf("expected cursor to move to 1 on 'j', got %d", editor.PathCursor)
	}

	// 'k' moves cursor up
	editor.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	if editor.PathCursor != 0 {
		t.Fatalf("expected cursor to move to 0 on 'k', got %d", editor.PathCursor)
	}

	// 'd' deletes the highlighted item
	editor.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if len(editor.Paths) != 2 || editor.Paths[0] != "/bin/p2" {
		t.Fatalf("expected /bin/p1 to be deleted on 'd', got %v", editor.Paths)
	}
}
