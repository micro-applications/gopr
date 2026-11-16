package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPullStack(t *testing.T) {
	// main ← a (#1) ← b (#2) ← c (#3), with #4 from a fork based on c and
	// #5 based on b too: b ends the stack, which branches there.
	type pr struct {
		n          int
		base, head string
		fork       bool
	}
	prs := []pr{{1, "main", "a", false}, {2, "a", "b", false}, {3, "b", "c", false}, {4, "c", "c", true}, {5, "b", "d", false}}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var req struct {
			Query     string            `json:"query"`
			Variables map[string]string `json:"variables"`
		}
		json.Unmarshal(b, &req)
		below := strings.Contains(req.Query, "headRefName: $ref")
		var nodes []string
		for _, p := range prs {
			if below && p.head == req.Variables["ref"] || !below && p.base == req.Variables["ref"] {
				nodes = append(nodes, fmt.Sprintf(`{"number":%d,"title":"T%d","baseRefName":%q,"headRefName":%q,"isCrossRepository":%t}`, p.n, p.n, p.base, p.head, p.fork))
			}
		}
		fmt.Fprintf(w, `{"data":{"repository":{"pullRequests":{"nodes":[%s]}}}}`, strings.Join(nodes, ","))
	}))
	t.Cleanup(s.Close)
	withAPI(t, s.URL)
	c := &Client{Token: "t"}
	repo := Repo{"acme", "app"}
	pull := func(n int, base, head string) *PullRequest {
		return &PullRequest{Number: n, Title: fmt.Sprintf("T%d", n), Base: Branch{Ref: base}, Head: Branch{Ref: head, Label: "acme:" + head}}
	}
	st, err := c.PullStack(context.Background(), repo, pull(2, "a", "b"))
	if err != nil || st == nil {
		t.Fatalf("stack %+v %v", st, err)
	}
	var got []int
	for _, p := range st.Pulls {
		got = append(got, p.Number)
	}
	if fmt.Sprint(got) != "[1 2]" || st.Trunk != "main" || st.Index(2) != 1 {
		t.Errorf("stack %v onto %q", got, st.Trunk)
	}
	// Above a branch with several pull requests, there is no stack.
	if st, err := c.PullStack(context.Background(), repo, pull(3, "b", "c")); st != nil || err != nil {
		t.Errorf("above a branching %+v %v", st, err)
	}
	// Without #5, the stack goes on to #3, but not to the fork's #4.
	prs = prs[:4]
	st, err = c.PullStack(context.Background(), repo, pull(1, "main", "a"))
	if err != nil || st == nil || len(st.Pulls) != 3 || st.Index(1) != 0 {
		t.Errorf("from the bottom %+v %v", st, err)
	}

	// A pull request alone, and one from a fork, have no stack.
	prs = []pr{{1, "main", "a", false}}
	if st, err := c.PullStack(context.Background(), repo, pull(1, "main", "a")); st != nil || err != nil {
		t.Errorf("alone %+v %v", st, err)
	}
	fork := pull(1, "main", "a")
	fork.Head.Label = "bo:a"
	if st, err := c.PullStack(context.Background(), repo, fork); st != nil || err != nil {
		t.Errorf("fork %+v %v", st, err)
	}
}
