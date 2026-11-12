package github

import (
	"context"
	"sort"
	"strings"
	"time"
)

// Section is a list of the inbox.
type Section int

const (
	Mine   Section = iota // pull requests the viewer opened
	Review                // pull requests waiting on the viewer's review
)

// Scope limits the inbox to some owners.
type Scope struct {
	All  bool     // no filter by owner
	Orgs []string // organization logins, when not All
	User string   // the viewer's login to include personal repositories, or ""
}

// InboxQuery is the search query of a section, or "" when the scope selects nothing.
func InboxQuery(s Section, scope Scope, since time.Time) string {
	if !scope.All && len(scope.Orgs) == 0 && scope.User == "" {
		return ""
	}
	q := []string{"is:pr", "is:open"}
	if s == Review {
		q = append(q, "draft:false", "review-requested:@me")
	} else {
		q = append(q, "author:@me")
	}
	if !scope.All {
		orgs := append([]string(nil), scope.Orgs...)
		sort.Strings(orgs)
		for _, o := range orgs {
			q = append(q, "org:"+o)
		}
		if scope.User != "" {
			q = append(q, "user:"+scope.User)
		}
	}
	q = append(q, "created:>="+since.Format("2006-01-02"), "sort:created-desc")
	return strings.Join(q, " ")
}

// InboxPull is a pull request of the inbox with its repository.
type InboxPull struct {
	Summary
	Repo Repo
}

const searchPullsQuery = `
query SearchPullRequests($query: String!) {
  search(query: $query, type: ISSUE, first: 100) {
    issueCount
    nodes {
      ... on PullRequest {` + pullFields + `
        repository { nameWithOwner }
      }
    }
  }
}`

// SearchPulls runs a search for pull requests. total is the number of matches,
// which may exceed the pulls returned.
func (c *Client) SearchPulls(ctx context.Context, query string) (pulls []InboxPull, total int, err error) {
	var data struct {
		Search struct {
			IssueCount int `json:"issueCount"`
			Nodes      []struct {
				searchNode
				Repository struct {
					NameWithOwner string `json:"nameWithOwner"`
				} `json:"repository"`
			} `json:"nodes"`
		} `json:"search"`
	}
	if err := c.graphql(ctx, searchPullsQuery, map[string]any{"query": query}, &data); err != nil {
		return nil, 0, err
	}
	for _, n := range data.Search.Nodes {
		if n.Number == 0 {
			continue // not a pull request
		}
		owner, name, _ := strings.Cut(n.Repository.NameWithOwner, "/")
		repo := Repo{Owner: owner, Name: name}
		pulls = append(pulls, InboxPull{Summary: summaryOf(n.searchNode), Repo: repo})
	}
	return pulls, data.Search.IssueCount, nil
}

// ViewerOrgs returns the login of the token's account and the organizations it belongs to.
func (c *Client) ViewerOrgs(ctx context.Context) (login string, orgs []string, err error) {
	var data struct {
		Viewer struct {
			Login         string `json:"login"`
			Organizations struct {
				Nodes []struct {
					Login string `json:"login"`
				} `json:"nodes"`
			} `json:"organizations"`
		} `json:"viewer"`
	}
	const q = `query { viewer { login organizations(first: 100) { nodes { login } } } }`
	if err := c.graphql(ctx, q, nil, &data); err != nil {
		return "", nil, err
	}
	for _, o := range data.Viewer.Organizations.Nodes {
		orgs = append(orgs, o.Login)
	}
	sort.Slice(orgs, func(i, j int) bool { return strings.ToLower(orgs[i]) < strings.ToLower(orgs[j]) })
	return data.Viewer.Login, orgs, nil
}
