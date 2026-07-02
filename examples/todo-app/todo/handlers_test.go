package todo

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestHandlersCRUDWalkthrough(t *testing.T) {
	pool := testPool(t)
	srv := httptest.NewServer(NewServer(pool))
	defer srv.Close()

	post := func(path, body string) *http.Response {
		t.Helper()
		resp, err := http.Post(srv.URL+path, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	// create
	resp := post("/todos", `{"title":"ship the example"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: status %d", resp.StatusCode)
	}
	var created Todo
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	if created.Title != "ship the example" || created.Done {
		t.Fatalf("create: %+v", created)
	}

	// empty title rejected
	resp = post("/todos", `{"title":"  "}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty title: status %d", resp.StatusCode)
	}
	resp.Body.Close()

	// toggle
	resp = post("/todos/"+jsonID(created.ID)+"/toggle", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("toggle: status %d", resp.StatusCode)
	}
	var toggled Todo
	json.NewDecoder(resp.Body).Decode(&toggled)
	resp.Body.Close()
	if !toggled.Done {
		t.Fatal("toggle did not flip done")
	}

	// list
	resp, err := http.Get(srv.URL + "/todos")
	if err != nil {
		t.Fatal(err)
	}
	var list []Todo
	json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()
	if len(list) != 1 {
		t.Fatalf("list: %+v", list)
	}

	// delete + 404s
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/todos/"+jsonID(created.ID), nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: status %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = post("/todos/"+jsonID(created.ID)+"/toggle", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("toggle after delete: status %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = post("/todos/not-a-number/toggle", "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad id: status %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func jsonID(id int64) string { return strconv.FormatInt(id, 10) }
