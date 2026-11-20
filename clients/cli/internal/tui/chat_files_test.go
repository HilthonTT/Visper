package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	apisdk "github.com/hilthontt/visper/sdk/go"
)

func TestRoomFilesDialog(t *testing.T) {
	m, _ := ownedChatModel(t, "someone-else")
	tm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = tm.(model)
	m.state.chat.messages = []apisdk.MessageResponse{
		{ID: "m1", Content: "http://visper.test/api/v1/d/r/f1.enc#My%20Cat.png"},
	}
	m, _ = m.openRoomFiles()
	if !m.state.chat.files.loading || m.state.dialog.kind != dialogRoomFiles {
		t.Fatal("files dialog not opened")
	}
	now := time.Now()
	m, _ = m.roomFilesLoadedUpdate(roomFilesLoadedMsg{session: m.state.chat.session, files: []apisdk.FileResponse{
		{ID: "f1", Size: 2048, CreatedAt: now.Add(-time.Hour), Uploader: apisdk.FileUploader{ID: m.userID}},
		{ID: "f2", Size: 10, CreatedAt: now, Uploader: apisdk.FileUploader{ID: "bob", Username: "Bob"}},
	}})
	f := m.state.chat.files
	if f.loading || len(f.files) != 2 || f.files[0].ID != "f2" {
		t.Fatalf("files = %+v", f.files)
	}
	view := m.roomFilesView()
	for _, want := range []string{"My Cat.png", "encrypted file", "Bob", "you"} {
		if !strings.Contains(view, want) {
			t.Errorf("files view lacks %q:\n%s", want, view)
		}
	}

	// Bob's file isn't ours and we don't own the room.
	m, _ = m.roomFilesUpdate(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if m.state.dialog.kind != dialogRoomFiles {
		t.Fatal("offered to delete someone else's file")
	}
	// Our own file can be deleted, after confirming.
	m, _ = m.roomFilesUpdate(tea.KeyPressMsg{Code: tea.KeyDown})
	m, _ = m.roomFilesUpdate(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if m.state.dialog.kind != dialogDeleteFile || m.state.chat.files.deleteID != "f1" {
		t.Fatalf("delete dialog = %v for %q", m.state.dialog.kind, m.state.chat.files.deleteID)
	}
	m, _ = m.chatDialogUpdate(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if m.state.dialog.kind != dialogRoomFiles {
		t.Fatal("cancelling didn't go back to the list")
	}
}

func TestSharedFileRef(t *testing.T) {
	id, name, ok := sharedFileRef("https://x/api/v1/d/room/abc-123.enc#notes%20v2.png")
	if !ok || id != "abc-123" || name != "notes v2.png" {
		t.Fatalf("got %q %q %v", id, name, ok)
	}
	if _, _, ok := sharedFileRef("hello there"); ok {
		t.Fatal("plain text parsed as a file")
	}
}
