package trade

import (
	"path/filepath"
	"testing"
)

func TestSessionStoreSaveInvokesDurableHook(t *testing.T) {
	SetSessionSaveHook(nil)
	t.Cleanup(func() { SetSessionSaveHook(nil) })

	var saved SessionAccountData
	SetSessionSaveHook(func(_ AccountConfig, entry SessionAccountData) error {
		saved = entry
		return nil
	})

	store := NewSessionStore(filepath.Join(t.TempDir(), "session.json"))
	account := AccountConfig{Name: "primary", Username: "primary"}
	if err := store.Save(account, SessionAccountData{Cookie: "cookie", Token: "token"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if saved.AccountName != "primary" || saved.Cookie != "cookie" || saved.Token != "token" {
		t.Fatalf("saved session = %#v", saved)
	}
}
