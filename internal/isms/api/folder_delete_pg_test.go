package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
)

// Postgres-gated regression tests for #310: a folder created in the Documents
// UI had no way to be removed, in the UI or through the API. DELETE
// /documents/folders?path=<folder> now removes a folder that holds no
// documents. Requires a migrated Postgres, see id_resolution_pg_test.go's
// testServer for setup. Skipped when ISMS_TEST_DATABASE_URL is unset.

const folderDeleteActor = "admin@folder-delete.test"

func createFolderForTest(t *testing.T, s *Server, orgID int, path, title string) {
	t.Helper()
	body := `{"path":"` + path + `","title":"` + title + `"}`
	c, rec := reviewCtxForPath(orgID, http.MethodPost, "/api/v1/documents/folders", body, "admin", folderDeleteActor)
	if err := s.handleCreateFolder(c); err != nil {
		t.Fatalf("handleCreateFolder(%s): %v", path, err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("handleCreateFolder(%s): expected 201, got %d: %s", path, rec.Code, rec.Body.String())
	}
}

func deleteFolderForTest(s *Server, orgID int, path, role string) (int, string, error) {
	target := "/api/v1/documents/folders?path=" + url.QueryEscape(path)
	c, rec := reviewCtxForPath(orgID, http.MethodDelete, target, "", role, folderDeleteActor)
	err := s.handleDeleteFolder(c)
	return rec.Code, rec.Body.String(), err
}

// topFolderNames lists the folders GET /documents/all returns at the top level.
func topFolderNames(t *testing.T, s *Server, orgID int) []string {
	t.Helper()
	c, rec := reviewCtxForPath(orgID, http.MethodGet, "/api/v1/documents/all", "", "admin", folderDeleteActor)
	if err := s.handleListAllDocuments(c); err != nil {
		t.Fatalf("handleListAllDocuments: %v", err)
	}
	var resp struct {
		Data []struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding /documents/all: %v", err)
	}
	var names []string
	for _, f := range resp.Data {
		names = append(names, f.Name)
	}
	return names
}

func hasFolder(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}

func TestDeleteEmptyFolderThroughAPI(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "folder-delete")
	seedRepoForOrg(t, s, orgID, "folder-delete-doc")
	seedReviewUser(t, s, orgID, folderDeleteActor, "admin")

	createFolderForTest(t, s, orgID, "custom", "Custom")
	createFolderForTest(t, s, orgID, "custom/nested", "Nested")
	if names := topFolderNames(t, s, orgID); !hasFolder(names, "custom") {
		t.Fatalf("precondition: the new folder should be listed, got %v", names)
	}

	code, body, err := deleteFolderForTest(s, orgID, "custom", "admin")
	if err != nil {
		t.Fatalf("handleDeleteFolder: %v", err)
	}
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", code, body)
	}
	var resp map[string]string
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if resp["commit"] == "" {
		t.Errorf("response should carry the commit hash, got %v", resp)
	}

	if hasFolder(topFolderNames(t, s, orgID), "custom") {
		t.Fatal("the deleted folder is still listed by /documents/all")
	}

	st, err := s.storeForOrg(context.Background(), orgID)
	if err != nil {
		t.Fatalf("storeForOrg: %v", err)
	}
	head, err := st.HeadCommit()
	if err != nil {
		t.Fatalf("HeadCommit: %v", err)
	}
	if head.Hash.String() != resp["commit"] {
		t.Errorf("HEAD = %s, want the returned commit %s", head.Hash, resp["commit"])
	}
	if head.Author.Email != folderDeleteActor {
		t.Errorf("commit author = %s, want the acting user %s", head.Author.Email, folderDeleteActor)
	}

	acts, err := s.db.RecentActivity(context.Background(), orgID, 10)
	if err != nil {
		t.Fatalf("RecentActivity: %v", err)
	}
	found := false
	for _, a := range acts {
		if a.Action == "folder_deleted" && a.Actor == folderDeleteActor {
			found = true
		}
	}
	if !found {
		t.Error("deleting a folder should be recorded in the activity log")
	}
}

func TestDeleteFolderRefusals(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "folder-delete-refuse")
	// Puts a document at documents/test/.
	seedRepoForOrg(t, s, orgID, "folder-refuse-doc")
	seedReviewUser(t, s, orgID, folderDeleteActor, "admin")
	createFolderForTest(t, s, orgID, "custom", "Custom")

	cases := []struct {
		name string
		path string
		role string
		want int
	}{
		{"folder with a document", "test", "admin", http.StatusConflict},
		{"missing folder", "nope", "admin", http.StatusNotFound},
		{"document path, not a folder", "test/folder-refuse-doc.md", "admin", http.StatusNotFound},
		{"empty path", "", "admin", http.StatusBadRequest},
		{"parent traversal", "../custom", "admin", http.StatusBadRequest},
		{"inner traversal", "test/../custom", "admin", http.StatusBadRequest},
		{"contributor", "custom", "contributor", http.StatusForbidden},
		{"reader", "custom", "reader", http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := deleteFolderForTest(s, orgID, tc.path, tc.role)
			wantHTTPStatus(t, err, tc.want)
		})
	}

	// None of the refusals may have removed anything.
	names := topFolderNames(t, s, orgID)
	for _, want := range []string{"test", "custom"} {
		if !hasFolder(names, want) {
			t.Errorf("folder %q should still be listed after the refusals, got %v", want, names)
		}
	}
}
